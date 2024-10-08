package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

type UnlockFeeCreditOption func(Attributes *fc.UnlockFeeCreditAttributes)

func NewUnlockFC(t *testing.T, signer abcrypto.Signer, attr *fc.UnlockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewUnlockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(NewFeeCreditRecordID(t, signer)),
		testtransaction.WithAttributes(attr),
		testtransaction.WithTransactionType(fc.TransactionTypeUnlockFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
		testtransaction.WithAuthProof(fc.UnlockFeeCreditAuthProof{}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultUnlockFCAttr() *fc.UnlockFeeCreditAttributes {
	return &fc.UnlockFeeCreditAttributes{
		Counter: counter,
	}
}

func NewUnlockFCAttr(opts ...UnlockFeeCreditOption) *fc.UnlockFeeCreditAttributes {
	defaultTx := NewDefaultUnlockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithUnlockFCCounter(counter uint64) UnlockFeeCreditOption {
	return func(attr *fc.UnlockFeeCreditAttributes) {
		attr.Counter = counter
	}
}
