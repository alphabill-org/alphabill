package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testhttp "github.com/alphabill-org/alphabill/internal/testutils/http"
	"github.com/alphabill-org/alphabill/internal/testutils/net"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/stretchr/testify/require"
)

func TestMoneyBackendCLI(t *testing.T) {
	// create ab network
	_ = wlog.InitStdoutLogger(wlog.INFO)
	initialBill := &moneytx.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	abNet := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)
	alphabillNodeAddr := moneyPartition.Nodes[0].AddrGRPC

	// transfer initial bill to wallet pubkey
	pk := "0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3"
	pkBytes, _ := pubKeyHexToBytes(pk)
	initialBillValue := spendInitialBillWithFeeCredits(t, abNet, initialBill, pkBytes)

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
		httpRes, _ := testhttp.DoGetJson(fmt.Sprintf("http://%s/api/v1/balance?pubkey=%s", serverAddr, pk), res)
		return httpRes != nil && httpRes.StatusCode == 200 && res.Balance == initialBillValue
	}, test.WaitDuration, test.WaitTick)

	// verify /list-bills
	resListBills := &backend.ListBillsResponse{}
	httpRes, err := testhttp.DoGetJson(fmt.Sprintf("http://%s/api/v1/list-bills?pubkey=%s", serverAddr, pk), resListBills)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Len(t, resListBills.Bills, 1)
	b := resListBills.Bills[0]
	require.Equal(t, initialBillValue, b.Value)
	require.Equal(t, initialBill.ID, types.UnitID(b.Id))
	require.NotNil(t, b.TxHash)

	// verify proof
	resBlockProof := &wallet.Proof{}
	httpRes, err = testhttp.DoGetCbor(fmt.Sprintf("http://%s/api/v1/units/0x%s/transactions/0x%x/proof", serverAddr, initialBill.ID, b.TxHash), resBlockProof)
	require.NoError(t, err)
	require.EqualValues(t, 200, httpRes.StatusCode)
	require.Equal(t, resBlockProof.TxRecord.TransactionOrder.Hash(crypto.SHA256), b.TxHash)
}

func TestMoneyBackendConfig_DbFileParentDirsAreCreated(t *testing.T) {
	expectedFilePath := filepath.Join(t.TempDir(), "non-existent-dir", "my.db")
	c := &moneyBackendConfig{DbFile: expectedFilePath}
	_, err := c.GetDbFile()
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Dir(expectedFilePath)))
}
