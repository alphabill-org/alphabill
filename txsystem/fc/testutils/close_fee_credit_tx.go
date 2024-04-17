package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func NewCloseFC(t *testing.T, attr *fc.CloseFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewCloseFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeCloseFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

type CloseFCOption func(*fc.CloseFeeCreditAttributes) CloseFCOption

func NewCloseFCAttr(opts ...CloseFCOption) *fc.CloseFeeCreditAttributes {
	defaultTx := NewDefaultCloseFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func NewDefaultCloseFCAttr() *fc.CloseFeeCreditAttributes {
	return &fc.CloseFeeCreditAttributes{
		Amount:            amount,
		TargetUnitID:      unitID,
		TargetUnitCounter: targetUnitCounter,
	}
}

func WithCloseFCAmount(amount uint64) CloseFCOption {
	return func(tx *fc.CloseFeeCreditAttributes) CloseFCOption {
		tx.Amount = amount
		return nil
	}
}

func WithCloseFCTargetUnitID(targetUnitID []byte) CloseFCOption {
	return func(tx *fc.CloseFeeCreditAttributes) CloseFCOption {
		tx.TargetUnitID = targetUnitID
		return nil
	}
}

func WithCloseFCTargetUnitCounter(counter uint64) CloseFCOption {
	return func(tx *fc.CloseFeeCreditAttributes) CloseFCOption {
		tx.TargetUnitCounter = counter
		return nil
	}
}
