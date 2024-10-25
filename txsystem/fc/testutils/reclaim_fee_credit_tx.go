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
		testtransaction.WithTransactionType(fc.TransactionTypeReclaimFeeCredit),
		testtransaction.WithAuthProof(fc.ReclaimFeeCreditAuthProof{}),
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
	tx, err := (newCloseFC(t)).MarshalCBOR()
	require.NoError(t, err)
	txr := &types.TransactionRecord{Version: 1,
		TransactionOrder: tx,
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        10,
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
	return &fc.ReclaimFeeCreditAttributes{CloseFeeCreditProof: testblock.CreateTxRecordProof(t, txr, signer)}
}

type ReclaimFCOption func(*fc.ReclaimFeeCreditAttributes) ReclaimFCOption

func WithReclaimFCClosureProof(proof *types.TxRecordProof) ReclaimFCOption {
	return func(tx *fc.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.CloseFeeCreditProof = proof
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
		testtransaction.WithTransactionType(fc.TransactionTypeCloseFeeCredit),
	)
}
