package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

type LockFeeCreditOption func(Attributes *fc.LockFeeCreditAttributes)

func NewLockFC(t *testing.T, signer abcrypto.Signer, attr *fc.LockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewLockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(NewFeeCreditRecordID(t, signer)),
		testtransaction.WithAttributes(attr),
		testtransaction.WithTransactionType(fc.TransactionTypeLockFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
		testtransaction.WithAuthProof(fc.LockFeeCreditAuthProof{}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultLockFCAttr() *fc.LockFeeCreditAttributes {
	return &fc.LockFeeCreditAttributes{
		LockStatus: 1,
		Counter:    counter,
	}
}

func NewLockFCAttr(opts ...LockFeeCreditOption) *fc.LockFeeCreditAttributes {
	defaultTx := NewDefaultLockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithLockFCCounter(counter uint64) LockFeeCreditOption {
	return func(attr *fc.LockFeeCreditAttributes) {
		attr.Counter = counter
	}
}

func WithLockStatus(lockStatus uint64) LockFeeCreditOption {
	return func(attr *fc.LockFeeCreditAttributes) {
		attr.LockStatus = lockStatus
	}
}
