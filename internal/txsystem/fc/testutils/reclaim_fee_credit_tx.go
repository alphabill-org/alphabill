package testutils

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

func NewReclaimFC(t *testing.T, signer abcrypto.Signer, reclaimFCAttr *transactions.ReclaimFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if reclaimFCAttr == nil {
		reclaimFCAttr = NewReclaimFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
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
		Backlink:               backlink,
	}
}

type ReclaimFCOption func(*transactions.ReclaimFeeCreditAttributes) ReclaimFCOption

func WithReclaimFCBacklink(backlink []byte) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditAttributes) ReclaimFCOption {
		tx.Backlink = backlink
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
		Amount:             amount,
		TargetUnitID:       unitID,
		TargetUnitBacklink: backlink,
	}
	return testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeCloseFeeCredit),
	)
}
