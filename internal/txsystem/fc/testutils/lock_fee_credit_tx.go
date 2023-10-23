package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
)

type LockFeeCreditOption func(Attributes *transactions.LockFeeCreditAttributes)

func NewLockFC(t *testing.T, attr *transactions.LockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewLockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
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
