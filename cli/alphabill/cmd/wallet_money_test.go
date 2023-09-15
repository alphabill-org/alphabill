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
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	wallet "github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
)

const walletBaseDir = "wallet"

type (
	backendMockReturnConf struct {
		balance        uint64
		blockHeight    uint64
		billID         types.UnitID
		billValue      uint64
		billTxHash     string
		proofList      string
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

func TestWalletCreateCmd_invalidSeed(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	cmd.baseCmd.SetArgs(strings.Split("wallet create -s --wallet-location "+homeDir, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.EqualError(t, err, `invalid value "--wallet-location" for flag "seed" (mnemonic)`)
	require.False(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
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

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	am, homedir := createNewWallet(t)
	pubKey, _ := am.GetPublicKey(0)
	am.Close()

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 5e8})
	defer mockServer.Close()

	_, err := execCommand(homedir, "send --amount 10 --address "+hexutil.Encode(pubKey)+" --alphabill-api-uri "+addr.Host)
	require.ErrorIs(t, err, wallet.ErrInsufficientBalance)
}

func createMoneyPartition(t *testing.T, initialBill *money.InitialBill, nodeCount int) *testpartition.NodePartition {
	moneyPart, err := testpartition.NewPartition(t, nodeCount, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := money.NewTxSystem(
			money.WithSystemIdentifier(money.DefaultSystemIdentifier),
			money.WithHashAlgorithm(crypto.SHA256),
			money.WithInitialBill(initialBill),
			money.WithSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
				{
					SystemIdentifier: money.DefaultSystemIdentifier,
					T2Timeout:        defaultT2Timeout,
					FeeCreditBill: &genesis.FeeCreditBill{
						UnitId:         money.NewBillID(nil, []byte{2}),
						OwnerPredicate: script.PredicateAlwaysTrue(),
					},
				},
			}),
			money.WithDCMoneyAmount(10000),
			money.WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, money.DefaultSystemIdentifier)
	require.NoError(t, err)
	return moneyPart
}

func startAlphabill(t *testing.T, partitions []*testpartition.NodePartition) *testpartition.AlphabillNetwork {
	abNetwork, err := testpartition.NewAlphabillPartition(partitions)
	require.NoError(t, err)
	require.NoError(t, abNetwork.Start(t))

	t.Cleanup(func() {
		require.NoError(t, abNetwork.Close())
	})
	return abNetwork
}

func startPartitionRPCServers(t *testing.T, partition *testpartition.NodePartition) {
	for _, n := range partition.Nodes {
		n.AddrGRPC = startRPCServer(t, n.Node)
	}
}

func startRPCServer(t *testing.T, node *partition.Node) string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer, err := initRPCServer(node, &grpcServerConfiguration{
		Address:               listener.Addr().String(),
		MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
		MaxRecvMsgSize:        defaultMaxRecvMsgSize,
		MaxSendMsgSize:        defaultMaxSendMsgSize,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})

	go func() {
		require.NoError(t, grpcServer.Serve(listener), "gRPC server exited with error")
	}()

	return listener.Addr().String()
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

func createNewWallet(t *testing.T) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, walletBaseDir)
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("")
	require.NoError(t, err)
	return homeDir
}

func verifyStdout(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	joined := consoleWriter.String()
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
	args := fmt.Sprintf("wallet --home %s %s", homedir, command)
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	return outputWriter
}

type testConsoleWriter struct {
	lines []string
}

func (w *testConsoleWriter) String() string {
	return strings.Join(w.lines, "\n")
}

func (w *testConsoleWriter) Println(a ...any) {
	s := fmt.Sprintln(a...)
	w.lines = append(w.lines, s[:len(s)-1]) // remove newline
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
			case "/" + client.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case "/" + client.RoundNumberPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case "/api/v1/units/":
				if br.proofList != "" {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(br.proofList))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			case "/" + client.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBase64(br.billID), br.billValue, br.billTxHash)))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBase64(bytes []byte) string {
	return base64.StdEncoding.EncodeToString(bytes)
}
