package cmd

import (
	"context"
	"crypto"
	"fmt"
	"net"
	"os"
	"path"
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
	require.True(t, util.FileExists(path.Join(os.TempDir(), "wallet-test", "wallet", "wallet.db")))
	verifyStdout(t, outputWriter,
		"Creating new wallet...",
		"Wallet created successfully.",
	)
}

func TestWalletGetBalanceCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout := execCommand(t, homedir, "get-balance")
	verifyStdout(t, stdout, "#1 0", "Total 0")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	addAccount(t, "wallet-test")
	stdout := execCommand(t, homedir, "get-balance --key 2")
	verifyStdout(t, stdout, "#2 0")
	verifyStdoutNotExists(t, stdout, "Total 0")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout := execCommand(t, homedir, "get-balance --total")
	verifyStdout(t, stdout, "Total 0")
	verifyStdoutNotExists(t, stdout, "#1 0")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout := execCommand(t, homedir, "get-balance --key 1 --total")
	verifyStdout(t, stdout, "#1 0")
	verifyStdoutNotExists(t, stdout, "Total 0")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := createNewTestWallet(t)

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout := execCommand(t, homedir, "get-balance --quiet")
	verifyStdout(t, stdout, "#1 0")
	verifyStdout(t, stdout, "Total 0")

	// verify quiet with total
	stdout = execCommand(t, homedir, "get-balance --quiet --total")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "#1 0")

	// verify quiet with key
	stdout = execCommand(t, homedir, "get-balance --quiet --key 1")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "Total 0")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout = execCommand(t, homedir, "get-balance --quiet --key 1 --total")
	verifyStdout(t, stdout, "0")
	verifyStdoutNotExists(t, stdout, "#1 0")
}

func TestPubKeysCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout := execCommand(t, homedir, "get-pubkeys")
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

	w1 := createNewNamedWallet(t, "wallet-1", ":9543")
	w1PubKey, _ := w1.GetPublicKey(0)
	w1.Shutdown()

	w2 := createNewNamedWallet(t, "wallet-2", ":9543")
	w2PubKey, _ := w2.GetPublicKey(0)
	w2.Shutdown()

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := createInitialBillTransferTx(w1PubKey, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	waitForBalance(t, "wallet-1", initialBill.Value, 0)

	// send two transactions (two bills) to wallet-2
	stdout := execWalletCmd(t, "wallet-1", "send --amount 1 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, "wallet-1", "send --amount 1 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	verifyTotalBalance(t, "wallet-1", initialBill.Value-2)

	// verify wallet-2 received said bills
	waitForBalance(t, "wallet-2", 2, 0)

	// swap wallet-2 bills
	stdout = execWalletCmd(t, "wallet-2", "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// send wallet-2 bill back to wallet-1
	stdout = execWalletCmd(t, "wallet-2", "send --amount 1 --address "+hexutil.Encode(w1PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	verifyTotalBalance(t, "wallet-2", 1)

	// verify wallet-1 balance is increased
	waitForBalance(t, "wallet-1", initialBill.Value-1, 0)
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
	walletName := "wallet"
	w := createNewNamedWallet(t, walletName, ":9543")
	pubKey1, _ := w.GetPublicKey(0)
	w.Shutdown()

	pubKey2Hex := addAccount(t, walletName)
	pubKey3Hex := addAccount(t, walletName)

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, walletName, initialBill.Value, 0)

	// send two transactions (two bills) to wallet account 2
	stdout := execWalletCmd(t, walletName, "send --amount 1 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, walletName, "send --amount 1 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	verifyAccountBalance(t, walletName, initialBill.Value-2, 0)

	// verify account 2 balance is increased
	waitForBalance(t, walletName, 2, 1)

	// swap account 2 bills
	stdout = execWalletCmd(t, walletName, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// send account 2 bills to account 3
	stdout = execWalletCmd(t, walletName, "send --amount 2 --key 2 --address "+pubKey3Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalance(t, walletName, 2, 2)
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

func waitForBalance(t *testing.T, walletName string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, walletName, "sync")
		verifyStdout(t, stdout, "Wallet synchronized successfully.")

		stdout = execWalletCmd(t, walletName, "get-balance")
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
func addAccount(t *testing.T, walletName string) string {
	stdout := execWalletCmd(t, walletName, "add-key")
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

func createNewNamedWallet(t *testing.T, name string, addr string) *money.Wallet {
	walletDir := path.Join(os.TempDir(), name)
	_ = os.RemoveAll(walletDir)

	w, err := money.CreateNewWallet("", money.WalletConfig{
		DbPath: path.Join(walletDir, "wallet"),
		AlphabillClientConfig: client.AlphabillClientConfig{
			Uri:          addr,
			WaitForReady: false,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, w)

	t.Cleanup(func() {
		_ = os.RemoveAll(walletDir)
	})
	return w
}

func createNewTestWallet(t *testing.T) string {
	homeDir := setupTestHomeDir(t, "wallet-test")
	walletDir := path.Join(os.TempDir(), "wallet-test", "wallet")
	w, err := money.CreateNewWallet("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky", money.WalletConfig{
		DbPath: walletDir,
	})
	defer w.Shutdown()
	require.NoError(t, err)
	require.NotNil(t, w)
	t.Cleanup(func() {
		_ = os.RemoveAll(walletDir)
	})
	return homeDir
}

func verifyStdout(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.Contains(t, consoleWriter.lines, expectedLine)
	}
}

func verifyStdoutNotExists(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.lines, expectedLine)
	}
}

func execCommand(t *testing.T, homeDir, command string) *testConsoleWriter {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	return outputWriter
}

func execWalletCmd(t *testing.T, walletName string, command string) *testConsoleWriter {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	homeDir := path.Join(os.TempDir(), walletName)

	cmd := New()
	args := "wallet --home " + homeDir + " " + command
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
