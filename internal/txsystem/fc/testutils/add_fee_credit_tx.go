package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/stretchr/testify/require"
)

var (
	unitID               = test.NewUnitID(1)
	moneySystemID        = []byte{0, 0, 0, 0}
	systemID             = []byte{0, 0, 0, 0}
	nonce                = []byte{3}
	backlink             = []byte{4}
	bearer               = []byte{5}
	amount               = uint64(50)
	fee                  = uint64(1)
	maxFee               = uint64(2)
	earliestAdditionTime = uint64(0)
	latestAdditionTime   = uint64(10)
)

func NewAddFC(t *testing.T, signer abcrypto.Signer, attr *transactions.AddFeeCreditAttributes, opts ...testtransaction.Option) *transactions.AddFeeCreditWrapper {
	if attr == nil {
		attr = NewAddFCAttr(t, signer)
	}
	defaultTx := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
	)
	for _, opt := range opts {
		require.NoError(t, opt(defaultTx))
	}
	tx, err := transactions.NewFeeCreditTx(defaultTx)
	require.NoError(t, err)

	return tx.(*transactions.AddFeeCreditWrapper)
}

type AddFCOption func(*transactions.AddFeeCreditAttributes) AddFCOption

func NewAddFCAttr(t *testing.T, signer abcrypto.Signer, opts ...AddFCOption) *transactions.AddFeeCreditAttributes {
	defaultFCTx := &transactions.AddFeeCreditAttributes{}
	for _, opt := range opts {
		opt(defaultFCTx)
	}
	if defaultFCTx.FeeCreditTransfer == nil {
		defaultFCTx.FeeCreditTransfer = NewTransferFC(t, nil).Transaction
	}
	if defaultFCTx.FeeCreditTransferProof == nil {
		gtx, _ := transactions.NewFeeCreditTx(defaultFCTx.FeeCreditTransfer)
		defaultFCTx.FeeCreditTransferProof = testblock.CreateProof(t, gtx, signer, defaultFCTx.FeeCreditTransfer.UnitId)
	}
	return defaultFCTx
}

func WithFCOwnerCondition(ownerCondition []byte) AddFCOption {
	return func(tx *transactions.AddFeeCreditAttributes) AddFCOption {
		tx.FeeCreditOwnerCondition = ownerCondition
		return nil
	}
}

func WithTransferFCProof(proof *block.BlockProof) AddFCOption {
	return func(tx *transactions.AddFeeCreditAttributes) AddFCOption {
		tx.FeeCreditTransferProof = proof
		return nil
	}
}

func WithTransferFCTx(ttx *txsystem.Transaction) AddFCOption {
	return func(tx *transactions.AddFeeCreditAttributes) AddFCOption {
		tx.FeeCreditTransfer = ttx
		return nil
	}
}
