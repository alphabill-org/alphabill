package testutils

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var (
	unitID               = newUnitID(1)
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

func NewAddFC(t *testing.T, signer abcrypto.Signer, attr *fc.AddFeeCreditOrder, opts ...testtransaction.Option) *fc.AddFeeCreditWrapper {
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
	tx, err := fc.NewFeeCreditTx(defaultTx)
	require.NoError(t, err)

	return tx.(*fc.AddFeeCreditWrapper)
}

type AddFCOption func(*fc.AddFeeCreditOrder) AddFCOption

func NewAddFCAttr(t *testing.T, signer abcrypto.Signer, opts ...AddFCOption) *fc.AddFeeCreditOrder {
	defaultFCTx := &fc.AddFeeCreditOrder{}
	for _, opt := range opts {
		opt(defaultFCTx)
	}
	if defaultFCTx.FeeCreditTransfer == nil {
		defaultFCTx.FeeCreditTransfer = NewTransferFC(t, nil).Transaction
	}
	if defaultFCTx.FeeCreditTransferProof == nil {
		gtx, _ := fc.NewFeeCreditTx(defaultFCTx.FeeCreditTransfer)
		defaultFCTx.FeeCreditTransferProof = testblock.CreateProof(t, gtx, signer, defaultFCTx.FeeCreditTransfer.UnitId)
	}
	return defaultFCTx
}

func WithFCOwnerCondition(ownerCondition []byte) AddFCOption {
	return func(tx *fc.AddFeeCreditOrder) AddFCOption {
		tx.FeeCreditOwnerCondition = ownerCondition
		return nil
	}
}

func WithTransferFCProof(proof *block.BlockProof) AddFCOption {
	return func(tx *fc.AddFeeCreditOrder) AddFCOption {
		tx.FeeCreditTransferProof = proof
		return nil
	}
}

func WithTransferFCTx(ttx *txsystem.Transaction) AddFCOption {
	return func(tx *fc.AddFeeCreditOrder) AddFCOption {
		tx.FeeCreditTransfer = ttx
		return nil
	}
}

func newUnitID(n uint64) []byte {
	return util.Uint256ToBytes(uint256.NewInt(n))
}
