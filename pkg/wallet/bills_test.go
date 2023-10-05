package wallet

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
)

var (
	amount         uint64 = 42
	targetValue    uint64 = 100
	remainingValue uint64 = 50
)

func TestBillVerifyTransferTx(t *testing.T) {
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithPayloadType(money.PayloadTypeTransfer),
		testtransaction.WithAttributes(money.TransferAttributes{
			NewBearer:   test.RandomBytes(32),
			TargetValue: targetValue,
			Backlink:    test.RandomBytes(32),
		}))

	// test invalid value
	b := &Bill{Value: targetValue - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCTargetUnitID
	b = &Bill{Value: targetValue, DCTargetUnitID: []byte{0}}
	err = b.verifyTx(tx)
	require.NotErrorIs(t, err, ErrMissingDCTargetUnitID)

	// test invalid txHash
	b = &Bill{Value: targetValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifyDCTransferTx(t *testing.T) {
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithPayloadType(money.PayloadTypeTransDC),
		testtransaction.WithAttributes(money.TransferDCAttributes{
			Value:    targetValue,
			Backlink: test.RandomBytes(32),
		}))

	// test invalid value
	b := &Bill{Value: targetValue - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Value: targetValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrMissingDCTargetUnitID)

	// test invalid txHash
	b = &Bill{Value: targetValue, DCTargetUnitID: []byte{0}}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, DCTargetUnitID: []byte{0}, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySplitTransferTx_OldBill(t *testing.T) {
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithUnitId(money.NewBillID(nil, []byte{3})),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithPayloadType(money.PayloadTypeSplit),
		testtransaction.WithAttributes(money.SplitAttributes{
			TargetUnits: []*money.TargetUnit{
				{Amount: amount, OwnerCondition: test.RandomBytes(32)},
			},
			RemainingValue: remainingValue,
			Backlink:       test.RandomBytes(32),
		}))

	// test invalid value
	b := &Bill{Id: tx.TransactionOrder.UnitID(), Value: remainingValue - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Id: tx.TransactionOrder.UnitID(), Value: remainingValue, DCTargetUnitID: []byte{0}}
	err = b.verifyTx(tx)
	require.NotErrorIs(t, err, ErrMissingDCTargetUnitID)

	// test invalid txHash
	b = &Bill{Id: tx.TransactionOrder.UnitID(), Value: remainingValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Id: tx.TransactionOrder.UnitID(), Value: remainingValue, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySplitTransferTx_NewBill(t *testing.T) {
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithUnitId(money.NewBillID(nil, []byte{3})),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithPayloadType(money.PayloadTypeSplit),
		testtransaction.WithAttributes(money.SplitAttributes{
			TargetUnits: []*money.TargetUnit{
				{Amount: amount, OwnerCondition: test.RandomBytes(32)},
			},
			RemainingValue: remainingValue,
			Backlink:       test.RandomBytes(32),
		}))

	newUnitID := money.NewBillID(tx.TransactionOrder.UnitID(), tx.TransactionOrder.Hash(crypto.SHA256))

	// test invalid value
	b := &Bill{Id: newUnitID, Value: amount - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Id: newUnitID, Value: amount, DCTargetUnitID: []byte{0}}
	err = b.verifyTx(tx)
	require.NotErrorIs(t, err, ErrMissingDCTargetUnitID)

	// test invalid txHash
	b = &Bill{Id: newUnitID, Value: amount}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Id: newUnitID, Value: amount, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySwapTransferTx(t *testing.T) {
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithUnitId(money.NewBillID(nil, []byte{1})),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
		testtransaction.WithPayloadType(money.PayloadTypeSwapDC),
		testtransaction.WithAttributes(money.SwapDCAttributes{TargetValue: targetValue}),
	)

	// test invalid value
	b := &Bill{Value: targetValue - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Value: targetValue, DCTargetUnitID: []byte{0}}
	err = b.verifyTx(tx)
	require.NotErrorIs(t, err, ErrMissingDCTargetUnitID)

	// test invalid txHash
	b = &Bill{Value: targetValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerify_NotMoneyTxType(t *testing.T) {
	type notMoneyAttr struct {
		_              struct{} `cbor:",toarray"`
		OwnerCondition []byte
	}
	tx := testtransaction.NewTransactionRecord(t,
		testtransaction.WithUnitId(money.NewBillID(nil, []byte{1})),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
		testtransaction.WithPayloadType("not money"),
		testtransaction.WithAttributes(notMoneyAttr{}),
	)

	// test invalid type
	b := &Bill{Value: targetValue, TxHash: tx.TransactionOrder.Hash(crypto.SHA256)}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxType)
}
