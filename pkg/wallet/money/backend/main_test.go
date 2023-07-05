package backend

import (
	"context"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/client/clientmock"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

func TestWalletBackend_BillsCanBeIndexedByPredicates(t *testing.T) {
	// create wallet backend with mock abclient
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	billId1 := newUnitID(1)
	billId2 := newUnitID(2)
	pubkey1, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubkey2, _ := hexutil.Decode("0x02c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	bearer1 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey1))
	bearer2 := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey2))
	fcbID := newUnitID(101)
	fcb := &Bill{Id: fcbID, Value: 100}

	abclient := clientmock.NewMockAlphabillClient(
		clientmock.WithMaxBlockNumber(1),
		clientmock.WithBlocks(map[uint64]*types.Block{
			1: {
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
				Transactions: []*types.TransactionRecord{{
					TransactionOrder: &types.TransactionOrder{
						Payload: &types.Payload{
							SystemID:       moneySystemID,
							Type:           moneytx.PayloadTypeTransfer,
							UnitID:         billId1,
							Attributes:     transferTxAttr(hash.Sum256(pubkey1)),
							ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
						},
					},
					ServerMetadata: &types.ServerMetadata{ActualFee: 1},
				}},
			},
			2: {
				UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
				Transactions: []*types.TransactionRecord{{
					TransactionOrder: &types.TransactionOrder{
						Payload: &types.Payload{
							SystemID:       moneySystemID,
							Type:           moneytx.PayloadTypeTransfer,
							UnitID:         billId2,
							Attributes:     transferTxAttr(hash.Sum256(pubkey2)),
							ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
						},
					},
					ServerMetadata: &types.ServerMetadata{ActualFee: 1},
				}},
			},
		}))
	storage, err := createTestBillStore(t)
	require.NoError(t, err)

	err = storage.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	getBlockNumber := func() (uint64, error) { return storage.Do().GetBlockNumber() }

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		bp, err := NewBlockProcessor(storage, moneySystemID)
		require.NoError(t, err)
		err = runBlockSync(ctx, abclient.GetBlocks, getBlockNumber, 100, bp.ProcessBlock)
		require.ErrorIs(t, err, context.Canceled)
	}()

	// verify first unit is indexed
	require.Eventually(t, func() bool {
		bills, err := storage.Do().GetBills(bearer1)
		require.NoError(t, err)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)

	// serve block with transaction to new pubkey
	abclient.SetMaxBlockNumber(2)

	// verify new bill is indexed by pubkey
	require.Eventually(t, func() bool {
		bills, err := storage.Do().GetBills(bearer2)
		require.NoError(t, err)
		return len(bills) > 0
	}, test.WaitDuration, test.WaitTick)
}

func TestGetBills_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubkey))
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferAttributes{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))
	txHash := tx.Hash(gocrypto.SHA256)

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	// add bill to service
	service := &WalletBackend{store: store}
	b := &Bill{
		Id:             tx.UnitID(),
		Value:          txValue,
		TxHash:         txHash,
		OwnerPredicate: bearer,
	}
	err = store.Do().SetBill(b, &wallet.Proof{
		TxRecord: &types.TransactionRecord{TransactionOrder: tx},
		TxProof:  &types.TxProof{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}}},
	})
	require.NoError(t, err)

	// verify bill can be queried by id
	bill, err := service.GetBill(b.Id)
	require.NoError(t, err)
	require.Equal(t, b, bill)

	// verify bill can be queried by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func TestGetBills_SHA512_OK(t *testing.T) {
	txValue := uint64(100)
	pubkey := make([]byte, 32)
	bearer := script.PredicatePayToPublicKeyHashDefault(hash.Sum512(pubkey))
	tx := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferAttributes{
		TargetValue: txValue,
		NewBearer:   bearer,
	}))

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	// add sha512 owner condition bill to service
	service := &WalletBackend{store: store}
	b := &Bill{
		Id:             tx.UnitID(),
		Value:          txValue,
		OwnerPredicate: bearer,
	}
	err = store.Do().SetBill(b, nil)
	require.NoError(t, err)

	// verify bill can be queried by pubkey
	bills, err := service.GetBills(pubkey)
	require.NoError(t, err)
	require.Len(t, bills, 1)
	require.Equal(t, b, bills[0])
}

func TestStoreDCMetadata_OK(t *testing.T) {
	// create 3 dc txs (two with same nonce)
	nonce1 := test.RandomBytes(32)
	tx1Value := uint64(100)
	tx1 := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferDCAttributes{
		TargetValue: tx1Value,
		Nonce:       nonce1,
	}), testtransaction.WithPayloadType(moneytx.PayloadTypeTransDC))

	tx2Value := uint64(200)
	tx2 := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferDCAttributes{
		TargetValue: tx2Value,
		Nonce:       nonce1,
	}), testtransaction.WithPayloadType(moneytx.PayloadTypeTransDC))

	nonce2 := test.RandomBytes(32)
	tx3Value := uint64(500)
	tx3 := testtransaction.NewTransactionOrder(t, testtransaction.WithAttributes(&moneytx.TransferDCAttributes{
		TargetValue: tx3Value,
		Nonce:       nonce2,
	}), testtransaction.WithPayloadType(moneytx.PayloadTypeTransDC))

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	// call store metadata on txs
	service := &WalletBackend{store: store}
	err = service.storeDCMetadata([]*types.TransactionOrder{tx1, tx2, tx3})
	require.NoError(t, err)

	// verify metadata for both nonces is correct
	dcMetadata1, err := service.GetDCMetadata(nonce1)
	require.NoError(t, err)
	require.Equal(t, tx1Value+tx2Value, dcMetadata1.DCSum)
	require.Len(t, dcMetadata1.BillIdentifiers, 2)
	require.Contains(t, dcMetadata1.BillIdentifiers, tx1.UnitID())
	require.Contains(t, dcMetadata1.BillIdentifiers, tx2.UnitID())

	dcMetadata2, err := service.GetDCMetadata(nonce2)
	require.NoError(t, err)
	require.Equal(t, tx3Value, dcMetadata2.DCSum)
	require.Len(t, dcMetadata2.BillIdentifiers, 1)
	require.Contains(t, dcMetadata2.BillIdentifiers, tx3.UnitID())
}

func Test_extractOwnerFromProof(t *testing.T) {
	sig := test.RandomBytes(64)
	pubkey := test.RandomBytes(32)
	predicate := script.PredicateArgumentPayToPublicKeyHashDefault(sig, pubkey)
	b, owner := extractOwnerFromProof(predicate)
	require.True(t, b)
	require.EqualValues(t, pubkey, owner)
}
