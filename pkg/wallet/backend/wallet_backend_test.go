package backend

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletBackend_BillsCanBeIndexedByPubkeys(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := uint256.NewInt(1)
	billId1Bytes := billId1.Bytes32()
	pubKey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	billId2 := uint256.NewInt(2)
	billId2Bytes := billId2.Bytes32()
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")

	abclient := clientmock.NewMockAlphabillClient(1, map[uint64]*block.Block{
		1: {
			BlockNumber: 1,
			Transactions: []*txsystem.Transaction{{
				UnitId:                billId1Bytes[:],
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubKey1)),
			}},
		},
		2: {
			BlockNumber: 2,
			Transactions: []*txsystem.Transaction{{
				UnitId:                billId2Bytes[:],
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubkey2)),
			}},
		},
	})
	w := createWalletBackend(abclient)
	err := w.AddKey(pubKey1)
	require.NoError(t, err)

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := w.Start(ctx)
		require.NoError(t, err)
	}()

	// verify first pubkey is indexed
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(pubKey1, billId1)
		return ok
	}, test.WaitDuration, test.WaitTick)

	// add new pubkey to indexer
	err = w.AddKey(pubkey2)
	require.NoError(t, err)

	// and serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(pubkey2, billId2)
		return ok
	}, test.WaitDuration, test.WaitTick)
}

func createWalletBackend(abclient client.ABClient) *WalletBackend {
	storage := NewInmemoryBillStore()
	bp := NewBlockProcessor(storage)
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	return New(genericWallet, storage)
}
