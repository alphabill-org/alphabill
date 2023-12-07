package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

type UnlockFeeCreditOption func(Attributes *transactions.UnlockFeeCreditAttributes)

func NewUnlockFC(t *testing.T, attr *transactions.UnlockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewUnlockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeUnlockFeeCredit),
		testtransaction.WithClientMetadata(&types.ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
		}),
		testtransaction.WithOwnerProof(nil),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewDefaultUnlockFCAttr() *transactions.UnlockFeeCreditAttributes {
	return &transactions.UnlockFeeCreditAttributes{
		Backlink: backlink,
	}
}

func NewUnlockFCAttr(opts ...UnlockFeeCreditOption) *transactions.UnlockFeeCreditAttributes {
	defaultTx := NewDefaultUnlockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithUnlockFCBacklink(backlink []byte) UnlockFeeCreditOption {
	return func(attr *transactions.UnlockFeeCreditAttributes) {
		attr.Backlink = backlink
	}
}
