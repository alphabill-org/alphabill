package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

type UnlockFeeCreditOption func(Attributes *fc.UnlockFeeCreditAttributes)

func NewUnlockFC(t *testing.T, attr *fc.UnlockFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewUnlockFCAttr()
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeUnlockFeeCredit),
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

func NewDefaultUnlockFCAttr() *fc.UnlockFeeCreditAttributes {
	return &fc.UnlockFeeCreditAttributes{
		Backlink: backlink,
	}
}

func NewUnlockFCAttr(opts ...UnlockFeeCreditOption) *fc.UnlockFeeCreditAttributes {
	defaultTx := NewDefaultUnlockFCAttr()
	for _, opt := range opts {
		opt(defaultTx)
	}
	return defaultTx
}

func WithUnlockFCBacklink(backlink []byte) UnlockFeeCreditOption {
	return func(attr *fc.UnlockFeeCreditAttributes) {
		attr.Backlink = backlink
	}
}
