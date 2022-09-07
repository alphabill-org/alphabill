package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletBackendCli(t *testing.T) {
	// create ab network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 10000,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, defaultServerAddr)

	// transfer initial bill to wallet
	pubkeyHex := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pubkey1, _ := hexutil.Decode(pubkeyHex)
	transferInitialBillTx, err := createInitialBillTransferTx(pubkey1, initialBill.ID, initialBill.Value, 10000)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// start wallet-backend service
	serverAddr := "localhost:8888"
	outputWriter := &testConsoleWriter{}
	go func() {
		homeDir := setupTestHomeDir(t, "wallet-backend-test")
		cmd := New()
		args := fmt.Sprintf("wallet-backend --home %s start --server-addr %s --pubkeys %s", homeDir, serverAddr, pubkeyHex)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		err = cmd.addAndExecuteCommand(ctx)
		require.NoError(t, err)
		fmt.Println(outputWriter.lines)
	}()

	// wait for wallet-backend to index the transaction by verifying balance
	require.Eventually(t, func() bool {
		// verify balance
		res := &backend.BalanceResponse{}
		httpRes := doGet(t, fmt.Sprintf("http://%s/balance?pubkey=%s", serverAddr, pubkeyHex), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == initialBill.Value
	}, test.WaitDuration, test.WaitTick)

	// verify list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes := doGet(t, fmt.Sprintf("http://%s/list-bills?pubkey=%s", serverAddr, pubkeyHex), resListBills)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	require.EqualValues(t, initialBill.Value, resListBills.Bills[0].Value)
	require.EqualValues(t, initialBill.ID, resListBills.Bills[0].Id)

	// verify block-proof
	resBlockProof := &backend.BlockProofResponse{}
	httpRes = doGet(t, fmt.Sprintf("http://%s/block-proof?bill_id=%s", serverAddr, initialBill.ID.Hex()), resBlockProof)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.NotNil(t, resBlockProof.BlockProof)
}

func doGet(t *testing.T, url string, response interface{}) *http.Response {
	httpRes, err := http.Get(url)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	defer func() {
		_ = httpRes.Body.Close()
	}()
	resBytes, _ := ioutil.ReadAll(httpRes.Body)
	log.Info("GET %s response: %s", url, string(resBytes))
	err = json.NewDecoder(bytes.NewReader(resBytes)).Decode(response)
	require.NoError(t, err)
	return httpRes
}
