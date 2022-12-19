package cmd

import (
	"context"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletBackendCli(t *testing.T) {
	// create ab network
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	initialBillBytes32 := initialBill.ID.Bytes32()
	initialBillHex := hexutil.Encode(initialBillBytes32[:])
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, defaultServerAddr)

	// create trust base file
	homedir := setupTestHomeDir(t, "wallet-backend-test")
	trustBaseFilePath := path.Join(homedir, "trust-base.json")
	_ = createTrustBaseFile(trustBaseFilePath, network)

	// transfer initial bill to wallet
	pubkeyHex := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkey1, _ := hexutil.Decode(pubkeyHex)
	transferInitialBillTx, err := createInitialBillTransferTx(pubkey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// start wallet-backend service
	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%d", port)
	consoleWriter = &testConsoleWriter{}
	go func() {
		cmd := New()
		args := fmt.Sprintf("wallet-backend --home %s start --server-addr %s --pubkeys %s --trust-base-file %s", homedir, serverAddr, pubkeyHex, trustBaseFilePath)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		err = cmd.addAndExecuteCommand(ctx)
		require.NoError(t, err)
	}()

	// wait for wallet-backend to index the transaction by verifying balance
	require.Eventually(t, func() bool {
		// verify balance
		res := &backend.BalanceResponse{}
		httpRes, _ := testhttp.DoGet(fmt.Sprintf("http://%s/api/v1/balance?pubkey=%s", serverAddr, pubkeyHex), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == initialBill.Value
	}, test.WaitDuration, test.WaitTick)

	// verify /list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes, err := testhttp.DoGet(fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pubkeyHex), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	b := resListBills.Bills[0]
	require.Equal(t, initialBill.Value, b.Value)
	require.Equal(t, initialBillBytes32[:], b.Id)
	require.NotNil(t, b.TxHash)

	// verify /proof
	resBlockProof := &block.Bills{}
	httpRes, err = testhttp.DoGetProto(fmt.Sprintf("http://%s/api/v1/proof/%s?bill_id=%s", serverAddr, pubkeyHex, initialBillHex), resBlockProof)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resBlockProof.Bills, 1)
}

/*
 Test case:
 1) send initial bill to wallet key 1
 2) export bill from wallet
 3) index key 1 in wallet-backend
 4) import bill to wallet-backend
 5) verify list-bills shows imported bill
 6) download proof from wallet-backend
 7) import downloaded proof to a new wallet
*/
func TestFlowBillImportExportDownloadUpload(t *testing.T) {
	// create ab network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	initialBillID := util.Uint256ToBytes(initialBill.ID)
	initialBillIDHex := hexutil.Encode(initialBillID)
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, defaultServerAddr)

	// create trust base file
	backendHomedir := setupTestHomeDir(t, "wallet-backend-test")
	trustBaseFilePath := path.Join(backendHomedir, "trust-base.json")
	_ = createTrustBaseFile(trustBaseFilePath, network)

	// start wallet-backend service
	pubkey1Hex := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkey1, _ := hexutil.Decode(pubkey1Hex)
	// pubkey2Hex := "0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	// pubkey2, _ := hexutil.Decode(pubkey2Hex)

	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%d", port)
	consoleWriter = &testConsoleWriter{}
	go func() {
		cmd := New()
		args := fmt.Sprintf("wallet-backend --home %s start --server-addr %s --trust-base-file %s", backendHomedir, serverAddr, trustBaseFilePath)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		err = cmd.addAndExecuteCommand(ctx)
		require.NoError(t, err)
	}()

	// create wallet
	walletHomedir := createNewTestWallet(t)

	// 1. send initial bill to wallet account 1
	transferInitialBillTx, err := createInitialBillTransferTx(pubkey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)
	waitForBalance(t, walletHomedir, initialBill.Value, 0)

	// 2. export proof from wallet account 1
	exportFilePath := path.Join(walletHomedir, "bill-0x0000000000000000000000000000000000000000000000000000000000000001.json")
	stdout, err := execBillsCommand(walletHomedir, "export --output-path "+walletHomedir)
	require.NoError(t, err)
	require.Equal(t, stdout.lines[0], fmt.Sprintf("Exported bill(s) to: %s", exportFilePath))

	// 3. index key 1 in wallet-backend
	req := &backend.AddKeyRequest{Pubkey: pubkey1Hex}
	res := &backend.EmptyResponse{}
	httpRes, err := testhttp.DoPost(fmt.Sprintf("http://%s/api/v1/admin/add-key", serverAddr), req, res)
	require.NoError(t, err)
	require.Equal(t, 200, httpRes.StatusCode)

	// 4. import bill to wallet-backend
	reqImportBill, _ := block.ReadBillsFile(exportFilePath)
	url := fmt.Sprintf("http://%s/api/v1/proof/%s", serverAddr, pubkey1Hex)
	httpRes, err = testhttp.DoPostProto(url, reqImportBill, &backend.EmptyResponse{})
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)

	// 5. verify list-bills shows imported bill
	resListBills := &backend.ListBillsResponse{}
	httpRes, err = testhttp.DoGet(fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pubkey1Hex), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	for _, b := range resListBills.Bills {
		require.Equal(t, initialBillID, b.Id)
		require.Equal(t, initialBill.Value, b.Value)
		require.NotNil(t, b.TxHash)
	}

	// 6. download proof from wallet-backend
	resGetProof := &block.Bills{}
	httpRes, err = testhttp.DoGetProto(fmt.Sprintf("http://%s/api/v1/proof/%s?bill_id=%s", serverAddr, pubkey1Hex, initialBillIDHex), resGetProof)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	downloadedBillFile := path.Join(walletHomedir, "downloaded-bill.json")
	err = block.WriteBillsFile(downloadedBillFile, resGetProof)
	require.NoError(t, err)

	// 7. import downloaded proof to a same but new wallet
	wallet2Homedir := createNewTestWallet(t)
	stdout, err = execBillsCommand(wallet2Homedir, fmt.Sprintf("import --bill-file=%s --trust-base-file=%s", downloadedBillFile, trustBaseFilePath))
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], "Successfully imported bill(s).")

	// 7.1 verify imported bill can be listed
	stdout, err = execBillsCommand(wallet2Homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "#1 0x0000000000000000000000000000000000000000000000000000000000000001 10000")
}
