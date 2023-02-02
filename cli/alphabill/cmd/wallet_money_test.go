package cmd

import (
	"context"
	"crypto"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestWalletCreateCmd(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	args := "wallet --home " + homeDir + " create"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(os.TempDir(), "wallet-test", "wallet", "wallet.db")))
	require.True(t, util.FileExists(filepath.Join(os.TempDir(), "wallet-test", "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"Creating new wallet...",
		"Wallet created successfully.",
	)
}

func TestWalletCreateCmd_encrypt(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	pw := "123456"
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" create --pn "+pw, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(os.TempDir(), "wallet-test", "wallet", "wallet.db")))
	require.True(t, util.FileExists(filepath.Join(os.TempDir(), "wallet-test", "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"Creating new wallet...",
		"Wallet created successfully.",
	)

	// verify wallet is encrypted
	// failing case: missing password
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key", " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// failing case: wrong password
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn 123", " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// passing case:
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn "+pw, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
}

func TestWalletGetBalanceCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, _ := execCommand(homedir, "get-balance")
	verifyStdout(t, stdout, "#1 0", "Total 0")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	addAccount(t, homedir)
	stdout, _ := execCommand(homedir, "get-balance --key 2")
	verifyStdout(t, stdout, "#2 0")
	verifyStdoutNotExists(t, stdout, "Total 0")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, _ := execCommand(homedir, "get-balance --total")
	verifyStdout(t, stdout, "Total 0")
	verifyStdoutNotExists(t, stdout, "#1 0")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, _ := execCommand(homedir, "get-balance --key 1 --total")
	verifyStdout(t, stdout, "#1 0")
	verifyStdoutNotExists(t, stdout, "Total 0")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := createNewTestWallet(t)

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(homedir, "get-balance --quiet")
	verifyStdout(t, stdout, "#1 0")
	verifyStdout(t, stdout, "Total 0")

	// verify quiet with total
	stdout, _ = execCommand(homedir, "get-balance --quiet --total")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "#1 0")

	// verify quiet with key
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "Total 0")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1 --total")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "#1 0")
}

func TestPubKeysCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, _ := execCommand(homedir, "get-pubkeys")
	verifyStdout(t, stdout, "#1 0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet-1
wallet-1 sends two transactions to wallet-2
wallet-2 swaps received bills
wallet-2 sends transaction back to wallet-1
*/
func TestSendingMoneyBetweenWallets(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create 2 wallets
	err := wlog.InitStdoutLogger(wlog.INFO)
	require.NoError(t, err)

	w1, homedir1 := createNewWallet(t, ":9543")
	w1PubKey, _ := w1.GetAccountManager().GetPublicKey(0)
	w1.Shutdown()

	w2, homedir2 := createNewWallet(t, ":9543")
	w2PubKey, _ := w2.GetAccountManager().GetPublicKey(0)
	w2.Shutdown()

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := createInitialBillTransferTx(w1PubKey, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	waitForBalance(t, homedir1, initialBill.Value, 0)

	// send two transactions (two bills) to wallet-2
	stdout := execWalletCmd(t, homedir1, "send --amount 1 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir1, "send --amount 1 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	verifyTotalBalance(t, homedir1, initialBill.Value-2)

	// verify wallet-2 received said bills
	waitForBalance(t, homedir2, 2, 0)

	// swap wallet-2 bills
	stdout = execWalletCmd(t, homedir2, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// send wallet-2 bill back to wallet-1
	stdout = execWalletCmd(t, homedir2, "send --amount 1 --address "+hexutil.Encode(w1PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	verifyTotalBalance(t, homedir2, 1)

	// verify wallet-1 balance is increased
	waitForBalance(t, homedir1, initialBill.Value-1, 0)
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet account 1
add two accounts to wallet
wallet account 1 sends two transactions to wallet account 2
wallet runs dust collection
wallet account 2 sends transaction to account 3
*/
func TestSendingMoneyBetweenWalletAccounts(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 3 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	//walletName := "wallet"
	w, homedir := createNewWallet(t, ":9543")
	pubKey1, _ := w.GetAccountManager().GetPublicKey(0)
	w.Shutdown()

	pubKey2Hex := addAccount(t, homedir)
	pubKey3Hex := addAccount(t, homedir)

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, initialBill.Value, 0)

	// send two transactions (two bills) to wallet account 2
	stdout := execWalletCmd(t, homedir, "send --amount 1 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir, "send --amount 1 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	verifyAccountBalance(t, homedir, initialBill.Value-2, 0)

	// verify account 2 balance is increased
	waitForBalance(t, homedir, 2, 1)

	// swap account 2 bills
	stdout = execWalletCmd(t, homedir, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// send account 2 bills to account 3
	stdout = execWalletCmd(t, homedir, "send --amount 2 --key 2 --address "+pubKey3Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalance(t, homedir, 2, 2)
}

func TestSendWithoutWaitingForConfirmation(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 3 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	w, homedir := createNewWallet(t, ":9543")
	pubKey1, _ := w.GetAccountManager().GetPublicKey(0)
	w.Shutdown()

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, initialBill.Value, 0)

	// verify transaction is broadcasted immediately
	stdout := execWalletCmd(t, homedir, "send -w false --amount 100 --address 0x00000046eed43bde3361e1a9ab6d0082dd923f20464a11869f8ef266045cf38d98")
	verifyStdout(t, stdout, "Successfully sent transaction(s)")
}

func TestSendCmdOutputPathFlag(t *testing.T) {
	// setup network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet
	homedir := createNewTestWallet(t)
	pubKey1Hex := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubKey2Hex := addAccount(t, homedir)

	// transfer initial bill to wallet account 1
	pubKey1Bytes, _ := hexutil.Decode(pubKey1Hex)
	transferInitialBillTx, _ := createInitialBillTransferTx(pubKey1Bytes, initialBill.ID, initialBill.Value, 10000)
	err := network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify tx is received by wallet
	waitForBalance(t, homedir, initialBill.Value, 0)

	// send two transactions to wallet account 2 and verify the proof files
	stdout, _ := execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s", 1, pubKey2Hex, homedir))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s", 1, pubKey2Hex, homedir))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	// verify wallet-2 balance is increased
	waitForBalance(t, homedir, 2, 1)

	// verify wallet-2 send-all-balance outputs both bills
	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --key %d", 2, pubKey1Hex, homedir, 2))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], fmt.Sprintf("Transaction proof(s) saved to: %s", filepath.Join(homedir, "bills.json")))
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			crypto.SHA256,
			initialBill,
			10000,
			moneytx.SchemeOpts.TrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, []byte{0, 0, 0, 0})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})
	return network
}

func startRPCServer(t *testing.T, network *testpartition.AlphabillPartition, addr string) {
	// start rpc server for network.Nodes[0]
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	grpcServer, err := initRPCServer(network.Nodes[0], &grpcServerConfiguration{
		Address:               addr,
		MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
		MaxRecvMsgSize:        defaultMaxRecvMsgSize,
		MaxSendMsgSize:        defaultMaxSendMsgSize,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
}

func waitForBalance(t *testing.T, homedir string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, homedir, "sync")
		verifyStdout(t, stdout, "Wallet synchronized successfully.")

		stdout = execWalletCmd(t, homedir, "get-balance")
		for _, line := range stdout.lines {
			if line == fmt.Sprintf("#%d %d", accountIndex+1, expectedBalance) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func verifyTotalBalance(t *testing.T, walletName string, expectedBalance uint64) {
	stdout := execWalletCmd(t, walletName, "get-balance")
	verifyStdout(t, stdout, fmt.Sprintf("Total %d", expectedBalance))
}

func verifyAccountBalance(t *testing.T, walletName string, expectedBalance, accountIndex uint64) {
	stdout := execWalletCmd(t, walletName, "get-balance")
	verifyStdout(t, stdout, fmt.Sprintf("#%d %d", accountIndex+1, expectedBalance))
}

// addAccount calls "add-key" cli function on given wallet and returns the added pubkey hex
func addAccount(t *testing.T, homedir string) string {
	stdout := execWalletCmd(t, homedir, "add-key")
	for _, line := range stdout.lines {
		if strings.HasPrefix(line, "Added key #") {
			return line[13:]
		}
	}
	return ""
}

func createInitialBillTransferTx(pubKey []byte, billId *uint256.Int, billValue uint64, timeout uint64) (*txsystem.Transaction, error) {
	billId32 := billId.Bytes32()
	tx := &txsystem.Transaction{
		UnitId:                billId32[:],
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            script.PredicateArgumentEmpty(),
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &moneytx.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    nil,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createNewWallet(t *testing.T, addr string) (*money.Wallet, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	w, err := money.CreateNewWallet(am, "", money.WalletConfig{
		DbPath: walletDir,
		AlphabillClientConfig: client.AlphabillClientConfig{
			Uri:          addr,
			WaitForReady: false,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, w)
	return w, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	w, err := money.CreateNewWallet(am, "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky", money.WalletConfig{
		DbPath: walletDir,
	})
	defer w.Shutdown()
	require.NoError(t, err)
	require.NotNil(t, w)
	return homeDir
}

func verifyStdout(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	joined := strings.Join(consoleWriter.lines, "\n")
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}

func verifyStdoutNotExists(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.lines, expectedLine)
	}
}

func execCommand(homeDir, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func execWalletCmd(t *testing.T, homedir string, command string) *testConsoleWriter {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	//homeDir := path.Join(os.TempDir(), walletName)

	cmd := New()
	args := "wallet --home " + homedir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	return outputWriter
}

type testConsoleWriter struct {
	lines []string
}

func (w *testConsoleWriter) Println(a ...any) {
	s := fmt.Sprint(a...)
	w.lines = append(w.lines, s)
}

func (w *testConsoleWriter) Print(a ...any) {
	w.Println(a...)
}
