package backend

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestWalletBackend_BillsCanBeIndexedByPubkeys(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newUnitId(1)
	pubKey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	billId2 := newUnitId(2)
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")

	abclient := clientmock.NewMockAlphabillClient(1, map[uint64]*block.Block{
		1: {
			BlockNumber: 1,
			Transactions: []*txsystem.Transaction{{
				UnitId:                billId1,
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubKey1)),
			}},
		},
		2: {
			BlockNumber: 2,
			Transactions: []*txsystem.Transaction{{
				UnitId:                billId2,
				SystemId:              alphabillMoneySystemId,
				TransactionAttributes: testtransaction.CreateBillTransferTx(hash.Sum256(pubkey2)),
			}},
		},
	})
	w := createWalletBackend(t, abclient)
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
		ok, _ := w.store.ContainsBill(billId1)
		return ok
	}, test.WaitDuration, test.WaitTick)

	// add new pubkey to indexer
	err = w.AddKey(pubkey2)
	require.NoError(t, err)

	// and serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		ok, _ := w.store.ContainsBill(billId2)
		return ok
	}, test.WaitDuration, test.WaitTick)
}

func TestSetBill_OK(t *testing.T) {
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
	}))
	proof, verifiers := createBlockProofForTx(t, tx)

	service := New(nil, NewInmemoryBillStore(), verifiers)
	pubkey := []byte{0}
	b := &Bill{
		Id:     tx.UnitId,
		Value:  txValue,
		TxHash: []byte{},
		BlockProof: &BlockProof{
			BlockNumber: 1,
			Tx:          tx,
			Proof:       proof,
		},
	}
	err := service.SetBills(pubkey, b)
	require.NoError(t, err)

	// verify saved bill can be queried by id
	bill, err := service.GetBill(tx.UnitId)
	require.NoError(t, err)
	require.Equal(t, b, bill)

	// verify saved bill can be queryed by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func TestSetBill_InvalidProof_NOK(t *testing.T) {
	txValue := uint64(100)
	tx := testtransaction.NewTransaction(t, testtransaction.WithAttributes(&moneytx.TransferOrder{
		TargetValue: txValue,
	}))
	proof, verifiers := createBlockProofForTx(t, tx)
	proof.BlockHeaderHash = make([]byte, 32) // invalidate proof

	service := New(nil, NewInmemoryBillStore(), verifiers)
	err := service.SetBills([]byte{}, &Bill{
		Id:     tx.UnitId,
		Value:  txValue,
		TxHash: []byte{},
		BlockProof: &BlockProof{
			BlockNumber: 1,
			Tx:          tx,
			Proof:       proof,
		},
	})
	require.ErrorIs(t, err, block.ErrProofVerificationFailed)
}

func createWalletBackend(t *testing.T, abclient client.ABClient) *WalletBackend {
	storage := NewInmemoryBillStore()
	bp := NewBlockProcessor(storage)
	genericWallet := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()
	_, verifier := testsig.CreateSignerAndVerifier(t)
	return New(genericWallet, storage, map[string]crypto.Verifier{"test": verifier})
}
