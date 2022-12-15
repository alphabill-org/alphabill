package money

import (
	"crypto"
	"testing"

	util2 "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestBillVerifyTransferTx(t *testing.T) {
	tx := createTransferTxOrder(t)

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
	tx := createTransferDCTxOrder(t)

	// test invalid value
	b := &Bill{Value: targetValue - 1, IsDcBill: true}
	err := b.verifyTx(tx)
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
	tx := createSplitTxOrder(t)

	// test invalid value
	b := &Bill{Id: util.Uint256ToBytes(tx.UnitID()), Value: remainingValue - 1}
	err := b.verifyTx(tx)
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
	tx := createSplitTxOrder(t)
	newUnitID := util2.SameShardIDBytes(tx.UnitID(), tx.Hash(crypto.SHA256))

	// test invalid value
	b := &Bill{Id: newUnitID, Value: amount - 1}
	err := b.verifyTx(tx)
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
	tx := createSwapTxOrder(t, nil, nil)

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
