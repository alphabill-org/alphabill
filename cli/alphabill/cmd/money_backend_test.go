package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
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
	initialBillID := util.Uint256ToBytes(initialBill.ID)
	initialBillHex := hexutil.Encode(initialBillID)
	network := startMoneyPartition(t, initialBill)
	alphabillNodeAddr := network.Nodes[0].AddrGRPC

	// transfer initial bill to wallet pubkey
	pk := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	initialBillValue := spendInitialBillWithFeeCredits(t, network, initialBill, pk)

	// start wallet-backend service
	homedir := setupTestHomeDir(t, "money-backend-test")
	port, err := net.GetFreePort()
	require.NoError(t, err)
	serverAddr := fmt.Sprintf("localhost:%d", port)
	consoleWriter = &testConsoleWriter{}
	go func() {
		cmd := New()
		args := fmt.Sprintf("money-backend --home %s start --server-addr %s --%s %s", homedir, serverAddr, alphabillNodeURLCmdName, alphabillNodeAddr)
		cmd.baseCmd.SetArgs(strings.Split(args, " "))

		ctx, cancelFunc := context.WithCancel(context.Background())
		t.Cleanup(cancelFunc)
		err = cmd.addAndExecuteCommand(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()

	// wait for wallet-backend to index the transaction by verifying balance
	require.Eventually(t, func() bool {
		// verify balance
		res := &backend.BalanceResponse{}
		httpRes, _ := testhttp.DoGet(fmt.Sprintf("http://%s/api/v1/balance?pubkey=%s", serverAddr, pk), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == initialBillValue
	}, test.WaitDuration, test.WaitTick)

	// verify /list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes, err := testhttp.DoGet(fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pk), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	b := resListBills.Bills[0]
	require.Equal(t, initialBillValue, b.Value)
	require.Equal(t, initialBillID, b.Id)
	require.NotNil(t, b.TxHash)

	// verify /proof
	resBlockProof := &bp.Bills{}
	httpRes, err = testhttp.DoGetProto(fmt.Sprintf("http://%s/api/v1/proof?bill_id=%s", serverAddr, initialBillHex), resBlockProof)
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
