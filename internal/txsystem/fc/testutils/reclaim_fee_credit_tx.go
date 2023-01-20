package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewReclaimFC(t *testing.T, signer abcrypto.Signer, reclaimFCAttr *fc.ReclaimFeeCreditOrder, opts ...testtransaction.Option) *fc.ReclaimFeeCreditWrapper {
	if reclaimFCAttr == nil {
		reclaimFCAttr = NewReclFCAttr(t, signer)
	}
	defaultReclFC := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(reclaimFCAttr),
	)
	for _, opt := range opts {
		require.NoError(t, opt(defaultReclFC))
	}
	tx, _ := fc.NewFeeCreditTx(defaultReclFC)
	return tx.(*fc.ReclaimFeeCreditWrapper)
}

func NewReclFCAttr(t *testing.T, signer abcrypto.Signer, opts ...ReclFCOption) *fc.ReclaimFeeCreditOrder {
	defaultReclaimFC := NewDefaultReclFCAttr(t, signer)
	for _, opt := range opts {
		opt(defaultReclaimFC)
	}
	return defaultReclaimFC
}

func NewDefaultReclFCAttr(t *testing.T, signer abcrypto.Signer) *fc.ReclaimFeeCreditOrder {
	closeFC := newCloseFC(t)
	closeFCProof := testblock.CreateProof(t, closeFC, signer, closeFC.Transaction.UnitId)
	return &fc.ReclaimFeeCreditOrder{
		CloseFeeCreditTransfer: closeFC.Transaction,
		CloseFeeCreditProof:    closeFCProof,
		Backlink:               backlink,
	}
}

type ReclFCOption func(*fc.ReclaimFeeCreditOrder) ReclFCOption

func WithReclFCBacklink(backlink []byte) ReclFCOption {
	return func(tx *fc.ReclaimFeeCreditOrder) ReclFCOption {
		tx.Backlink = backlink
		return nil
	}
}

func WithReclFCCloseFCProof(proof *block.BlockProof) ReclFCOption {
	return func(tx *fc.ReclaimFeeCreditOrder) ReclFCOption {
		tx.CloseFeeCreditProof = proof
		return nil
	}
}

func WithReclFCCloseFCTx(closeFCTx *txsystem.Transaction) ReclFCOption {
	return func(tx *fc.ReclaimFeeCreditOrder) ReclFCOption {
		tx.CloseFeeCreditTransfer = closeFCTx
		return nil
	}
}

func newCloseFC(t *testing.T) *fc.CloseFeeCreditWrapper {
	to := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
	)
	attr := &fc.CloseFeeCreditOrder{
		Amount:       amount,
		TargetUnitId: unitID,
		Nonce:        backlink,
	}
	_ = anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	tx, _ := fc.NewFeeCreditTx(to)
	return tx.(*fc.CloseFeeCreditWrapper)
}
