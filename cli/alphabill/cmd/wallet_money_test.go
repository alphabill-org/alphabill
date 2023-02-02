package cmd

import (
	"context"
	"crypto"
	"fmt"
	"net"
	"path"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
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
	require.True(t, util.FileExists(path.Join(homeDir, "wallet", "wallet.db")))
	verifyStdout(t, outputWriter,
		"Creating new wallet...",
		"Wallet created successfully.",
	)
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
	w1PubKey, _ := w1.GetPublicKey(0)
	w1.Shutdown()

	w2, homedir2 := createNewWallet(t, ":9543")
	w2PubKey, _ := w2.GetPublicKey(0)
	w2.Shutdown()

	// create fee credit for initial bill transfer
	txFee := moneytx.TxFeeFunc()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := createInitialBillTransferTx(w1PubKey, initialBill.ID, w1Balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	waitForBalance(t, homedir1, w1Balance, 0)

	// create fee credit for wallet-1
	amount := uint64(100)
	stdout := execWalletCmd(t, homedir1, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir1, w1Balance, 0)

	// send two transactions to wallet-2
	stdout = execWalletCmd(t, homedir1, "send --amount 50 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir1, "send --amount 150 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	w1Balance -= 200
	verifyAccountBalance(t, homedir1, w1Balance, 0)

	// verify wallet-2 received said bills
	w2Balance := uint64(200)
	waitForBalance(t, homedir2, w2Balance, 0)

	// create fee credit for wallet-2
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - amount - txFee
	waitForBalance(t, homedir2, w2Balance, 0)

	// swap wallet-2 bills
	stdout = execWalletCmd(t, homedir2, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// verify balance after swap remains the same
	verifyTotalBalance(t, homedir2, w2Balance)

	// send wallet-2 bill back to wallet-1
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("send --amount %d --address %s", w2Balance, hexutil.Encode(w1PubKey)))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	verifyTotalBalance(t, homedir2, 0)

	// verify wallet-1 balance is increased
	w1Balance += w2Balance
	waitForBalance(t, homedir1, w1Balance, 0)
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
	w, homedir := createNewWallet(t, ":9543")
	pubKey1, _ := w.GetPublicKey(0)
	w.Shutdown()

	pubKey2Hex := addAccount(t, homedir)
	pubKey3Hex := addAccount(t, homedir)

	// create fee credit for initial bill transfer
	txFee := moneytx.TxFeeFunc()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value-fcrAmount-txFee, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, w1Balance, 0)

	// create fee credit for wallet-1
	amount := uint64(100)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir, w1Balance, 0)

	// send two transactions to wallet account 2
	stdout = execWalletCmd(t, homedir, "send --amount 50 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir, "send --amount 150 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	w1Balance -= 200
	verifyAccountBalance(t, homedir, w1Balance, 0)

	// verify account 2 balance is increased
	w2Balance := uint64(200)
	waitForBalance(t, homedir, w2Balance, 1)

	// create fee credit for wallet-2
	w2FeeCredit := uint64(10)
	stdout = execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d -k 2", w2FeeCredit))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", w2FeeCredit))

	// verify balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - w2FeeCredit - txFee
	waitForBalance(t, homedir, w2Balance, 1)

	// swap account 2 bills
	stdout = execWalletCmd(t, homedir, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// verify balance after swap remains the same
	verifyAccountBalance(t, homedir, w2Balance, 1)

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
	pubKey1, _ := w.GetPublicKey(0)
	w.Shutdown()

	// create fee credit for initial bill transfer
	txFee := moneytx.TxFeeFunc()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, balance, 0)

	// create fee credit for wallet account 1
	amount := uint64(10)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	balance = balance - amount - txFee
	verifyAccountBalance(t, homedir, balance, 0)

	// verify transaction is broadcasted immediately
	stdout = execWalletCmd(t, homedir, "send -w false --amount 100 --address 0x00000046eed43bde3361e1a9ab6d0082dd923f20464a11869f8ef266045cf38d98")
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

	// create fee credit for initial bill transfer
	txFee := moneytx.TxFeeFunc()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet account 1
	pubKey1Bytes, _ := hexutil.Decode(pubKey1Hex)
	transferInitialBillTx, _ := createInitialBillTransferTx(pubKey1Bytes, initialBill.ID, w1Balance, 10000, initialBillBacklink)
	err := network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify tx is received by wallet
	waitForBalance(t, homedir, w1Balance, 0)

	// create fee credit for wallet-1
	amount := uint64(10)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir, w1Balance, 0)

	// send two transactions to wallet account 2 and verify the proof files
	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s", 5, pubKey2Hex, homedir))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s", 5, pubKey2Hex, homedir))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	// verify wallet-2 balance is increased
	w2Balance := uint64(10)
	waitForBalance(t, homedir, w2Balance, 1)

	// create fee credit for wallet-2
	w2Fees := uint64(3)
	stdout = execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d -k 2", w2Fees))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", w2Fees))

	// verify wallet-2 balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - w2Fees - txFee
	verifyAccountBalance(t, homedir, w2Balance, 1)

	// verify wallet-2 send-all-balance outputs both bills
	stdout, err = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --key %d", w2Balance, pubKey1Hex, homedir, 2))
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], fmt.Sprintf("Transaction proof(s) saved to: %s", path.Join(homedir, "bills.json")))
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			crypto.SHA256,
			initialBill,
			[]*genesis.SystemDescriptionRecord{
				{
					SystemIdentifier: defaultABMoneySystemIdentifier,
					T2Timeout:        defaultT2Timeout,
					FeeCreditBill: &genesis.FeeCreditBill{
						UnitId:         util.Uint256ToBytes(uint256.NewInt(2)),
						OwnerPredicate: script.PredicateAlwaysTrue(),
					},
				},
			},
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

func createInitialBillTransferTx(pubKey []byte, billId *uint256.Int, billValue uint64, timeout uint64, backlink []byte) (*txsystem.Transaction, error) {
	billId32 := billId.Bytes32()
	tx := &txsystem.Transaction{
		UnitId:                billId32[:],
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           timeout,
			MaxFee:            moneytx.TxFeeFunc(),
			FeeCreditRecordId: util.Uint256ToBytes(testmoney.FCRID),
		},
	}
	err := anypb.MarshalFrom(tx.TransactionAttributes, &moneytx.TransferOrder{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func createNewWallet(t *testing.T, addr string) (*money.Wallet, string) {
	walletDir := t.TempDir()
	w, err := money.CreateNewWallet("", money.WalletConfig{
		DbPath: path.Join(walletDir, "wallet"),
		AlphabillClientConfig: client.AlphabillClientConfig{
			Uri:          addr,
			WaitForReady: false,
		},
	})
	require.NoError(t, err)
	require.NotNil(t, w)
	return w, walletDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := path.Join(homeDir, "wallet")
	w, err := money.CreateNewWallet("dinosaur simple verify deliver bless ridge monkey design venue six problem lucky", money.WalletConfig{
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
	s := fmt.Sprintln(a...)
	w.lines = append(w.lines, s[:len(s)-1]) // remove newline
}

func (w *testConsoleWriter) Print(a ...any) {
	w.Println(a...)
}
