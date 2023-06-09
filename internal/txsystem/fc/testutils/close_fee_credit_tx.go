package testutils

import (
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func NewCloseFC(t *testing.T, attr *transactions.CloseFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewCloseFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeCloseFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

type CloseFCOption func(*transactions.CloseFeeCreditAttributes) CloseFCOption

func NewCloseFCAttr(opts ...CloseFCOption) *transactions.CloseFeeCreditAttributes {
	defaultTx := NewDefaultCloseFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func NewDefaultCloseFCAttr() *transactions.CloseFeeCreditAttributes {
	return &transactions.CloseFeeCreditAttributes{
		Amount:       amount,
		TargetUnitID: unitID,
		Nonce:        nonce,
	}
}

func WithCloseFCAmount(amount uint64) CloseFCOption {
	return func(tx *transactions.CloseFeeCreditAttributes) CloseFCOption {
		tx.Amount = amount
		return nil
	}
}

func WithCloseFCTargetUnitID(targetUnitID []byte) CloseFCOption {
	return func(tx *transactions.CloseFeeCreditAttributes) CloseFCOption {
		tx.TargetUnitID = targetUnitID
		return nil
	}
}

func WithCloseFCNonce(nonce []byte) CloseFCOption {
	return func(tx *transactions.CloseFeeCreditAttributes) CloseFCOption {
		tx.Nonce = nonce
		return nil
	}
}
