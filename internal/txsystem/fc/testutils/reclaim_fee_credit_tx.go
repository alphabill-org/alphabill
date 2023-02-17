package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func NewReclaimFC(t *testing.T, signer abcrypto.Signer, reclaimFCAttr *transactions.ReclaimFeeCreditOrder, opts ...testtransaction.Option) *transactions.ReclaimFeeCreditWrapper {
	if reclaimFCAttr == nil {
		reclaimFCAttr = NewReclaimFCAttr(t, signer)
	}
	defaultReclaimFC := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(reclaimFCAttr),
	)
	for _, opt := range opts {
		require.NoError(t, opt(defaultReclaimFC))
	}
	tx, _ := transactions.NewFeeCreditTx(defaultReclaimFC)
	return tx.(*transactions.ReclaimFeeCreditWrapper)
}

func NewReclaimFCAttr(t *testing.T, signer abcrypto.Signer, opts ...ReclaimFCOption) *transactions.ReclaimFeeCreditOrder {
	defaultReclaimFC := NewDefaultReclaimFCAttr(t, signer)
	for _, opt := range opts {
		opt(defaultReclaimFC)
	}
	return defaultReclaimFC
}

func NewDefaultReclaimFCAttr(t *testing.T, signer abcrypto.Signer) *transactions.ReclaimFeeCreditOrder {
	closeFC := newCloseFC(t)
	closeFCProof := testblock.CreateProof(t, closeFC, signer, closeFC.Transaction.UnitId)
	return &transactions.ReclaimFeeCreditOrder{
		CloseFeeCreditTransfer: closeFC.Transaction,
		CloseFeeCreditProof:    closeFCProof,
		Backlink:               backlink,
	}
}

type ReclaimFCOption func(*transactions.ReclaimFeeCreditOrder) ReclaimFCOption

func WithReclaimFCBacklink(backlink []byte) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditOrder) ReclaimFCOption {
		tx.Backlink = backlink
		return nil
	}
}

func WithReclaimFCClosureProof(proof *block.BlockProof) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditOrder) ReclaimFCOption {
		tx.CloseFeeCreditProof = proof
		return nil
	}
}

func WithReclaimFCClosureTx(closeFCTx *txsystem.Transaction) ReclaimFCOption {
	return func(tx *transactions.ReclaimFeeCreditOrder) ReclaimFCOption {
		tx.CloseFeeCreditTransfer = closeFCTx
		return nil
	}
}

func newCloseFC(t *testing.T) *transactions.CloseFeeCreditWrapper {
	to := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
	)
	attr := &transactions.CloseFeeCreditOrder{
		Amount:       amount,
		TargetUnitId: unitID,
		Nonce:        backlink,
	}
	_ = anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	tx, _ := transactions.NewFeeCreditTx(to)
	return tx.(*transactions.CloseFeeCreditWrapper)
}
