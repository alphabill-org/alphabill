package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

type LockFeeCreditOption func(Attributes *transactions.LockFeeCreditAttributes)

func NewLockFC(t *testing.T, attr *transactions.LockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewLockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeLockFeeCredit),
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

func NewDefaultLockFCAttr() *transactions.LockFeeCreditAttributes {
	return &transactions.LockFeeCreditAttributes{
		LockStatus: 1,
		Backlink:   backlink,
	}
}

func NewLockFCAttr(opts ...LockFeeCreditOption) *transactions.LockFeeCreditAttributes {
	defaultTx := NewDefaultLockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithLockFCBacklink(backlink []byte) LockFeeCreditOption {
	return func(attr *transactions.LockFeeCreditAttributes) {
		attr.Backlink = backlink
	}
}

func WithLockStatus(lockStatus uint64) LockFeeCreditOption {
	return func(attr *transactions.LockFeeCreditAttributes) {
		attr.LockStatus = lockStatus
	}
}
