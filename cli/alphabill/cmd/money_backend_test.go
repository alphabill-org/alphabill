package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
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
	backend "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestMoneyBackendCLI(t *testing.T) {
	// create ab network
	_ = wlog.InitStdoutLogger(wlog.INFO)
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	initialBillBytes32 := initialBill.ID.Bytes32()
	initialBillHex := hexutil.Encode(initialBillBytes32[:])
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
	homedir := setupTestHomeDir(t, "money-backend-test")
	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%d", port)
	consoleWriter = &testConsoleWriter{}
	go func() {
		cmd := New()
		args := fmt.Sprintf("money-backend --home %s start --server-addr %s", homedir, serverAddr)
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
		httpRes := testhttp.DoGet(t, fmt.Sprintf("http://%s/api/v1/balance?pubkey=%s", serverAddr, pubkeyHex), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == strconv.FormatUint(initialBill.Value, 10)
	}, test.WaitDuration, test.WaitTick)

	// verify /list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes := testhttp.DoGet(t, fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pubkeyHex), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	b := resListBills.Bills[0]
	require.Equal(t, strconv.FormatUint(initialBill.Value, 10), b.Value)
	require.Equal(t, initialBillBytes32[:], b.Id)
	require.NotNil(t, b.TxHash)

	// verify /proof
	resBlockProof := &block.Bills{}
	httpRes = testhttp.DoGetProto(t, fmt.Sprintf("http://%s/api/v1/proof?bill_id=%s", serverAddr, initialBillHex), resBlockProof)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resBlockProof.Bills, 1)
}

func TestMoneyBackendConfig_DbFileParentDirsAreCreated(t *testing.T) {
	expectedFilePath := filepath.Join(t.TempDir(), "non-existent-dir", "my.db")
	c := &moneyBackendConfig{DbFile: expectedFilePath}
	_, err := c.GetDbFile()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Dir(expectedFilePath)))
}
