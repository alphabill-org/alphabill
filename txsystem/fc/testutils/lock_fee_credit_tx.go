package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

type LockFeeCreditOption func(Attributes *fc.LockFeeCreditAttributes)

func NewLockFC(t *testing.T, attr *fc.LockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewLockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeLockFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultLockFCAttr() *fc.LockFeeCreditAttributes {
	return &fc.LockFeeCreditAttributes{
		LockStatus: 1,
		Backlink:   backlink,
	}
}

func NewLockFCAttr(opts ...LockFeeCreditOption) *fc.LockFeeCreditAttributes {
	defaultTx := NewDefaultLockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithLockFCBacklink(backlink []byte) LockFeeCreditOption {
	return func(attr *fc.LockFeeCreditAttributes) {
		attr.Backlink = backlink
	}
}

func WithLockStatus(lockStatus uint64) LockFeeCreditOption {
	return func(attr *fc.LockFeeCreditAttributes) {
		attr.LockStatus = lockStatus
	}
}
