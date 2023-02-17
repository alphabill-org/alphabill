package bp

import (
	"crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

var (
	amount         uint64 = 42
	targetValue    uint64 = 100
	remainingValue uint64 = 50
)

func TestBillVerifyTransferTx(t *testing.T) {
	tx := money.NewTransferTx(t, targetValue, test.RandomBytes(32), test.RandomBytes(32))

	// test invalid value
	b := &Bill{Value: targetValue - 1}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Value: targetValue, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidDCBillFlag)

	// test invalid txHash
	b = &Bill{Value: targetValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, TxHash: tx.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifyDCTransferTx(t *testing.T) {
	tx, err := money.ConvertNewGenericMoneyTx(money.CreateRandomDcTx())
	require.NoError(t, err)

	// test invalid value
	b := &Bill{Value: targetValue - 1, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Value: targetValue, IsDcBill: false}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidDCBillFlag)

	// test invalid txHash
	b = &Bill{Value: targetValue, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, IsDcBill: true, TxHash: tx.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySplitTransferTx_OldBill(t *testing.T) {
	tx, err := money.ConvertNewGenericMoneyTx(&txsystem.Transaction{
		UnitId:                test.NewUnitID(3),
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: money.CreateBillSplitTx(test.RandomBytes(32), targetValue, remainingValue),
	})
	require.NoError(t, err)

	// test invalid value
	b := &Bill{Id: util.Uint256ToBytes(tx.UnitID()), Value: remainingValue - 1}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Id: util.Uint256ToBytes(tx.UnitID()), Value: remainingValue, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidDCBillFlag)

	// test invalid txHash
	b = &Bill{Id: util.Uint256ToBytes(tx.UnitID()), Value: remainingValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Id: util.Uint256ToBytes(tx.UnitID()), Value: remainingValue, TxHash: tx.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySplitTransferTx_NewBill(t *testing.T) {
	tx, err := money.ConvertNewGenericMoneyTx(&txsystem.Transaction{
		UnitId:                test.NewUnitID(3),
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: money.CreateBillSplitTx(test.RandomBytes(32), amount, remainingValue),
	})
	require.NoError(t, err)

	newUnitID := txutil.SameShardIDBytes(tx.UnitID(), tx.Hash(crypto.SHA256))

	// test invalid value
	b := &Bill{Id: newUnitID, Value: amount - 1}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Id: newUnitID, Value: amount, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidDCBillFlag)

	// test invalid txHash
	b = &Bill{Id: newUnitID, Value: amount}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Id: newUnitID, Value: amount, TxHash: tx.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerifySwapTransferTx(t *testing.T) {
	tx, err := money.ConvertNewGenericMoneyTx(testtransaction.NewTransaction(t,
		testtransaction.WithAttributes(money.CreateRandomSwapAttributes(t, 0)),
		testtransaction.WithUnitId([]byte{0, 0, 0, 1}),
		testtransaction.WithSystemID([]byte{0, 0, 0, 0}),
		testtransaction.WithOwnerProof([]byte{0, 0, 0, 2}),
	))
	require.NoError(t, err)

	// test invalid value
	b := &Bill{Value: targetValue - 1}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidValue)

	// test invalid DCBillFlag
	b = &Bill{Value: targetValue, IsDcBill: true}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidDCBillFlag)

	// test invalid txHash
	b = &Bill{Value: targetValue}
	err = b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxHash)

	// test ok
	b = &Bill{Value: targetValue, TxHash: tx.Hash(crypto.SHA256)}
	err = b.verifyTx(tx)
	require.NoError(t, err)
}

func TestBillVerify_NotMoneyTxType(t *testing.T) {
	tx := testtransaction.NewGenericTransaction(t, txsystem.NewDefaultGenericTransaction)

	// test invalid type
	b := &Bill{Value: targetValue, TxHash: tx.Hash(crypto.SHA256)}
	err := b.verifyTx(tx)
	require.ErrorIs(t, err, ErrInvalidTxType)
}
