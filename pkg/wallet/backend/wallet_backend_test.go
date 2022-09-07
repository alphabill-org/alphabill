package backend

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletBackend_BillCanBeIndexed(t *testing.T) {
	pubKey, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	billId := uint256.NewInt(1)
	billIdBytes := billId.Bytes32()
	abclient := clientmock.NewMockAlphabillClient(1, map[uint64]*block.Block{
		1: {
			BlockNumber: 1,
			Transactions: []*txsystem.Transaction{{
				UnitId:                billIdBytes[:],
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubKey)),
			}},
		},
	})
	w := New([][]byte{pubKey}, abclient, NewInmemoryBillStore())
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := w.Start(ctx)
		require.NoError(t, err)
	}()
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(pubKey, billId)
		return ok
	}, test.WaitDuration, test.WaitTick)
}
