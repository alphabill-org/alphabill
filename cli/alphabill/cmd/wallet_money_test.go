package cmd

import (
	"context"
	"crypto"
	"encoding/base64"
	"fmt"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	testclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

type (
	backendMockReturnConf struct {
		balance        uint64
		blockHeight    uint64
		billId         *uint256.Int
		billValue      uint64
		billTxHash     string
		customBillList string
		customPath     string
		customFullPath string
		customResponse string
	}
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
	require.True(t, util.FileExists(filepath.Join(homeDir, "account", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
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
	require.True(t, util.FileExists(filepath.Join(homeDir, "account", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")

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
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15})
	defer mockServer.Close()
	addAccount(t, homedir)
	stdout, _ := execCommand(homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#2 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Total 15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15})
	defer mockServer.Close()

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(homedir, "get-balance --quiet --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(homedir, "get-balance --quiet --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestPubKeysCmd(t *testing.T) {
	am, homedir := createNewWallet(t)
	pk, _ := am.GetPublicKey(0)
	am.Close()
	stdout, _ := execCommand(homedir, "get-pubkeys")
	verifyStdout(t, stdout, "#1 "+hexutil.Encode(pk))
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet-1
wallet-1 sends two transactions to wallet-2
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

	am1, homedir1 := createNewWallet(t)
	w1PubKey, _ := am1.GetPublicKey(0)
	am1.Close()

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := createInitialBillTransferTx(w1PubKey, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	txHash, blockNumber := getLastTransactionProps(network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: txHash})
	defer mockServer.Close()

	// verify bill is received by wallet 1
	waitForBalance(t, homedir1, initialBill.Value, 0)

	// send two transactions (two bills) to wallet-2
	stdout := execWalletCmd(t, homedir1, "send --amount 1 --address "+hexutil.Encode(w2PubKey)+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir1, "send --amount 1 --address "+hexutil.Encode(w2PubKey)+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	verifyTotalBalance(t, homedir1, initialBill.Value-2)

	// verify wallet-2 received said bills
	waitForBalance(t, homedir2, 2, 0)
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet account 1
add two accounts to wallet
wallet account 1 sends two transactions to wallet account 2
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
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()

	pubKey2Hex := addAccount(t, homedir)
	_ = addAccount(t, homedir)

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, initialBill.Value, 0)

	txHash, blockNumber := getLastTransactionProps(network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: txHash})
	defer mockServer.Close()

	// send two transactions (two bills) to wallet account 2
	stdout := execWalletCmd(t, homedir, "send --amount 1 --address "+pubKey2Hex+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir, "send --amount 1 --address "+pubKey2Hex+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	verifyAccountBalance(t, homedir, initialBill.Value-2, 0)

	// verify account 2 balance is increased
	waitForBalance(t, homedir, 2, 1)
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
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, initialBill.Value, 0)

	gb, bp, _ := network.GetBlockProof(transferInitialBillTx, txConverter)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: gb.BlockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: base64.StdEncoding.EncodeToString(bp.TransactionsHash)})
	defer mockServer.Close()

	// verify transaction is broadcasted immediately
	stdout := execWalletCmd(t, homedir, "send -w false --amount 100 --address 0x00000046eed43bde3361e1a9ab6d0082dd923f20464a11869f8ef266045cf38d98 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully sent transaction(s)")
}

func TestSendCmdOutputPathFlag(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 2 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	//walletName := "wallet"
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()

	pubKey2Hex := addAccount(t, homedir)

	// transfer initial bill to wallet
	transferInitialBillTx, err := createInitialBillTransferTx(pubKey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, initialBill.Value, 0)

	txHash, blockNumber := getLastTransactionProps(network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: txHash})
	defer mockServer.Close()

	// send two transactions to wallet account 2 and verify the proof files
	stdout, _ := execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --alphabill-api-uri %s", 1, pubKey2Hex, homedir, addr.Host))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --alphabill-api-uri %s", 1, pubKey2Hex, homedir, addr.Host))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")
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
		mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
		defer mockServer.Close()

		stdout := execWalletCmd(t, homedir, "get-balance --alphabill-api-uri "+mockAddress.Host)
		for _, line := range stdout.lines {
			if line == fmt.Sprintf("#%d %d", accountIndex+1, expectedBalance) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func verifyTotalBalance(t *testing.T, walletName string, expectedBalance uint64) {
	mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
	defer mockServer.Close()
	stdout := execWalletCmd(t, walletName, "get-balance --alphabill-api-uri "+mockAddress.Host)
	verifyStdout(t, stdout, fmt.Sprintf("Total %d", expectedBalance))
}

func verifyAccountBalance(t *testing.T, walletName string, expectedBalance, accountIndex uint64) {
	mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
	defer mockServer.Close()
	stdout := execWalletCmd(t, walletName, "get-balance --alphabill-api-uri "+mockAddress.Host)
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

func createNewWallet(t *testing.T) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "account")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "account")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("")
	require.NoError(t, err)
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
	s := fmt.Sprint(a...)
	w.lines = append(w.lines, s)
}

func (w *testConsoleWriter) Print(a ...any) {
	w.Println(a...)
}

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			switch r.URL.Path {
			case "/" + testclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": %d}`, br.balance)))
			case "/" + testclient.BlockHeightPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": %d}`, br.blockHeight)))
			case "/" + testclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":%d,"txHash":"%s","isDCBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBillId(i *uint256.Int) string {
	return base64.StdEncoding.EncodeToString(util.Uint256ToBytes(i))
}

func txConverter(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, tx)
}

func getLastTransactionProps(network *testpartition.AlphabillPartition) (string, uint64) {
	gb, bp, _ := network.GetBlockProof(network.Nodes[0].GetLatestBlock().Transactions[0], txConverter)
	txHash := base64.StdEncoding.EncodeToString(bp.GetTransactionsHash())
	return txHash, gb.BlockNumber
}
