package backend

import (
	"context"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestWalletBackend_CanBeStarted(t *testing.T) {
	pubKey, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	abclient := clientmock.NewMockAlphabillClient(1, map[uint64]*block.Block{
		1: {
			BlockNumber: 1,
			Transactions: []*txsystem.Transaction{{
				UnitId:                newUnitId(1),
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubKey)),
			}},
		},
	})
	w := New([][]byte{pubKey}, abclient, NewInmemoryBillStore())
	ctx, cancelFunc := context.WithTimeout(context.Background(), time.Second)
	defer cancelFunc()
	go func() {
		err := w.Start(ctx)
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		bills, err := w.GetBills(pubKey)
		require.NoError(t, err)
		require.Len(t, bills, 1)
		return true
	}, test.WaitDuration, test.WaitTick)
}
