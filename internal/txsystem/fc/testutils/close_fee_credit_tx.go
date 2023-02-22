package testutils

import (
	"testing"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/stretchr/testify/require"
)

func NewCloseFC(t *testing.T, attr *transactions.CloseFeeCreditAttributes, opts ...testtransaction.Option) *transactions.CloseFeeCreditWrapper {
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
	tx, err := transactions.NewFeeCreditTx(defaultTx)
	require.NoError(t, err)

	return tx.(*transactions.CloseFeeCreditWrapper)
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
		TargetUnitId: unitID,
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
		tx.TargetUnitId = targetUnitID
		return nil
	}
}

func WithCloseFCNonce(nonce []byte) CloseFCOption {
	return func(tx *transactions.CloseFeeCreditAttributes) CloseFCOption {
		tx.Nonce = nonce
		return nil
	}
}
