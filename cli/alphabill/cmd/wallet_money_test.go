package cmd

import (
	"context"
	"crypto"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	netutil "github.com/alphabill-org/alphabill/internal/testutils/net"
	indexer "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"

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
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
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
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
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
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	addAccount(t, homedir)
	stdout, _ := execCommand(homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#2 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Total 15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
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
Prep: start network and money backend, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: when sending a tx, wallet-1 specifies --output-path flag and checks proofs are saved there
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 3: wallet-2 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	// create 2 wallets
	am1, homedir1 := createNewWallet(t)
	acc1, _ := am1.GetAccountKey(0)
	w1PubKey := acc1.PubKey
	am1.Close()

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	nodePort := netutil.GetFreeRandomPort(t)
	startRPCServer(t, network, ":"+strconv.Itoa(nodePort))
	nodeAddr := fmt.Sprintf("localhost:%v", nodePort)

	err := wlog.InitStdoutLogger(wlog.INFO)
	require.NoError(t, err)

	apiAddr := startMoneyBackend(t, nodeAddr)

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := createInitialBillTransferTx(w1PubKey, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(initialBill.Value, 8)))

	// TS1:
	// send two transactions (two bills) to wallet-2
	stdout := execWalletCmd(t, homedir1, fmt.Sprintf("send --amount 1 --address 0x%x --alphabill-api-uri %s --alphabill-uri %s", w2PubKey, apiAddr, nodeAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// TS1.1: also verify --output-path flag
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 1 --address 0x%x --alphabill-api-uri %s --alphabill-uri %s --output-path %s", w2PubKey, apiAddr, nodeAddr, homedir1))
	proofFile := fmt.Sprintf("%s/bill-0x0000000000000000000000000000000000000000000000000000000000000001.json", homedir1)
	verifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		fmt.Sprintf("Transaction proof(s) saved to: %s", proofFile),
	)
	require.FileExists(t, proofFile)

	// verify wallet-1 balance is decreased
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(initialBill.Value-2e8, 8)))

	// verify wallet-2 received said bills
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(2e8, 8)))

	// TS2:
	// add additional accounts to wallet 1
	pubKey2Hex := addAccount(t, homedir1)
	_ = addAccount(t, homedir1)
	// send two transactions (two bills) to wallet account 2
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 1 --address %s --alphabill-api-uri %s --alphabill-uri %s", pubKey2Hex, apiAddr, nodeAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 1 --address %s --alphabill-api-uri %s --alphabill-uri %s", pubKey2Hex, apiAddr, nodeAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance -k 1 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(initialBill.Value-4e8, 8)))

	// verify account 2 received said bills
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 2, amountToString(2e8, 8)))

	// TS3:
	// verify transaction is broadcasted immediately
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("send -w false --amount 2 --address 0x%x --alphabill-api-uri %s --alphabill-uri %s", w1PubKey, apiAddr, nodeAddr))
	verifyStdout(t, stdout, "Successfully sent transaction(s)")
}

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	am, homedir := createNewWallet(t)
	pubKey, _ := am.GetPublicKey(0)
	am.Close()

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 5e8})
	defer mockServer.Close()

	_, err := execCommand(homedir, "send --amount 10 --address "+hexutil.Encode(pubKey)+" --alphabill-api-uri "+addr.Host)
	require.ErrorIs(t, err, money.ErrInsufficientBalance)
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

func startMoneyBackend(t *testing.T, nodeAddr string) string {
	apiPort := netutil.GetFreeRandomPort(t)
	apiAddr := fmt.Sprintf("localhost:%v", apiPort)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	go func() {
		err := indexer.CreateAndRun(ctx, &indexer.Config{
			ABMoneySystemIdentifier: defaultABMoneySystemIdentifier,
			AlphabillUrl:            nodeAddr,
			ServerAddr:              apiAddr,
			DbFile:                  fmt.Sprintf("%s/backend.db", t.TempDir()),
			ListBillsPageLimit:      100,
		})
		wlog.Info("backend stopped")
		require.NoError(t, err)
	}()

	return apiAddr
}

func waitForBalance(t *testing.T, homedir string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
		defer mockServer.Close()

		stdout := execWalletCmd(t, homedir, "get-balance --alphabill-api-uri "+mockAddress.Host)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
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
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
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
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case "/" + testclient.BlockHeightPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case "/" + testclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
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
