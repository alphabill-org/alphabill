package testutils

import (
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/stretchr/testify/require"
)

func NewCloseFC(t *testing.T, attr *fc.CloseFeeCreditOrder, opts ...testtransaction.Option) *fc.CloseFeeCreditWrapper {
	if attr == nil {
		attr = NewCloseFCAttr()
	}
	defaultTx := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
	)
	for _, opt := range opts {
		require.NoError(t, opt(defaultTx))
	}
	tx, err := fc.NewFeeCreditTx(defaultTx)
	require.NoError(t, err)

	return tx.(*fc.CloseFeeCreditWrapper)
}

type CloseFCOption func(*fc.CloseFeeCreditOrder) CloseFCOption

func NewCloseFCAttr(opts ...CloseFCOption) *fc.CloseFeeCreditOrder {
	defaultTx := NewDefaultCloseFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func NewDefaultCloseFCAttr() *fc.CloseFeeCreditOrder {
	return &fc.CloseFeeCreditOrder{
		Amount:       amount,
		TargetUnitId: unitID,
		Nonce:        nonce,
	}
}

func WithCloseFCAmount(amount uint64) CloseFCOption {
	return func(tx *fc.CloseFeeCreditOrder) CloseFCOption {
		tx.Amount = amount
		return nil
	}
}

func WithCloseFCTargetUnitID(targetUnitID []byte) CloseFCOption {
	return func(tx *fc.CloseFeeCreditOrder) CloseFCOption {
		tx.TargetUnitId = targetUnitID
		return nil
	}
}

func WithCloseFCNonce(nonce []byte) CloseFCOption {
	return func(tx *fc.CloseFeeCreditOrder) CloseFCOption {
		tx.Nonce = nonce
		return nil
	}
}
