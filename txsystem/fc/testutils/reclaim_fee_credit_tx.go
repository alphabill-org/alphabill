package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func NewReclaimFC(t *testing.T, signer abcrypto.Signer, reclaimFCAttr *fc.ReclaimFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if reclaimFCAttr == nil {
		reclaimFCAttr = NewReclaimFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(DefaultMoneyUnitID()),
		testtransaction.WithAttributes(reclaimFCAttr),
		testtransaction.WithPayloadType(fc.PayloadTypeReclaimFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewReclaimFCAttr(t *testing.T, signer abcrypto.Signer, opts ...ReclaimFCOption) *fc.ReclaimFeeCreditAttributes {
	defaultReclaimFC := NewDefaultReclaimFCAttr(t, signer)
	for _, opt := range opts {
		opt(defaultReclaimFC)
	}
	return defaultReclaimFC
}

func NewDefaultReclaimFCAttr(t *testing.T, signer abcrypto.Signer) *fc.ReclaimFeeCreditAttributes {
	tr := &types.TransactionRecord{
		TransactionOrder: newCloseFC(t),
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        10,
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
	return &fc.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: tr,
		CloseFeeCreditProof:    testblock.CreateProof(t, tr, signer),
		Counter:                counter,
	}
}

type ReclaimFCOption func(*fc.ReclaimFeeCreditAttributes) ReclaimFCOption

func WithReclaimFCCounter(counter uint64) ReclaimFCOption {
	return func(tx *fc.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.Counter = counter
		return nil
	}
}

func WithReclaimFCClosureProof(proof *types.TxProof) ReclaimFCOption {
	return func(tx *fc.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.CloseFeeCreditProof = proof
		return nil
	}
}

func WithReclaimFCClosureTx(closeFCTx *types.TransactionRecord) ReclaimFCOption {
	return func(tx *fc.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.CloseFeeCreditTransfer = closeFCTx
		return nil
	}
}

func newCloseFC(t *testing.T) *types.TransactionOrder {
	attr := &fc.CloseFeeCreditAttributes{
		Amount:            amount,
		TargetUnitID:      DefaultMoneyUnitID(),
		TargetUnitCounter: targetCounter,
	}
	return testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(DefaultMoneyUnitID()),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeCloseFeeCredit),
	)
}
