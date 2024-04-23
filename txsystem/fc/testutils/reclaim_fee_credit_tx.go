package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func NewReclaimFC(t *testing.T, signer abcrypto.Signer, reclaimFCAttr *transactions.ReclaimFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if reclaimFCAttr == nil {
		reclaimFCAttr = NewReclaimFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(reclaimFCAttr),
		testtransaction.WithPayloadType(transactions.PayloadTypeReclaimFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

func NewReclaimFCAttr(t *testing.T, signer abcrypto.Signer, opts ...ReclaimFCOption) *transactions.ReclaimFeeCreditAttributes {
	defaultReclaimFC := NewDefaultReclaimFCAttr(t, signer)
	for _, opt := range opts {
		opt(defaultReclaimFC)
	}
	return defaultReclaimFC
}

func NewDefaultReclaimFCAttr(t *testing.T, signer abcrypto.Signer) *transactions.ReclaimFeeCreditAttributes {
	tr := &types.TransactionRecord{
		TransactionOrder: newCloseFC(t),
		ServerMetadata: &types.ServerMetadata{
			ActualFee: 10,
		},
	}
	return &transactions.ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: tr,
		CloseFeeCreditProof:    testblock.CreateProof(t, tr, signer),
		Counter:                counter,
	}
}

type ReclaimFCOption func(*transactions.ReclaimFeeCreditAttributes) ReclaimFCOption

func WithReclaimFCCounter(counter uint64) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.Counter = counter
		return nil
	}
}

func WithReclaimFCClosureProof(proof *types.TxProof) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.CloseFeeCreditProof = proof
		return nil
	}
}

func WithReclaimFCClosureTx(closeFCTx *types.TransactionRecord) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.CloseFeeCreditTransfer = closeFCTx
		return nil
	}
}

func newCloseFC(t *testing.T) *types.TransactionOrder {
	attr := &transactions.CloseFeeCreditAttributes{
		Amount:            amount,
		TargetUnitID:      unitID,
		TargetUnitCounter: targetCounter,
	}
	return testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeCloseFeeCredit),
	)
}
