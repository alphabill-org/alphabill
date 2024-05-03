package testutils

import (
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	unitID                              = types.NewUnitID(33, nil, []byte{1}, []byte{0xff}) // TODO: should be a parameter from a partition
	systemID             types.SystemID = 1
	targetUnitCounter                   = uint64(3)
	backlink                            = []byte{4}
	targetCounter                       = uint64(4)
	counter                             = uint64(4)
	amount                              = uint64(50)
	maxFee                              = uint64(2)
	earliestAdditionTime                = uint64(0)
	latestAdditionTime                  = uint64(10)
)

func NewAddFC(t *testing.T, signer abcrypto.Signer, attr *fc.AddFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewAddFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeAddFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

type AddFeeCreditOption func(*fc.AddFeeCreditAttributes) AddFeeCreditOption

func NewAddFCAttr(t *testing.T, signer abcrypto.Signer, opts ...AddFeeCreditOption) *fc.AddFeeCreditAttributes {
	defaultFCTx := &fc.AddFeeCreditAttributes{}
	for _, opt := range opts {
		opt(defaultFCTx)
	}
	if defaultFCTx.FeeCreditTransfer == nil {
		defaultFCTx.FeeCreditTransfer = &types.TransactionRecord{
			TransactionOrder: NewTransferFC(t, nil),
			ServerMetadata:   &types.ServerMetadata{},
		}
	}
	if defaultFCTx.FeeCreditTransferProof == nil {
		defaultFCTx.FeeCreditTransferProof = testblock.CreateProof(t, defaultFCTx.FeeCreditTransfer, signer)
	}
	return defaultFCTx
}

func WithFCOwnerCondition(ownerCondition []byte) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditOwnerCondition = ownerCondition
		return nil
	}
}

func WithTransferFCProof(proof *types.TxProof) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransferProof = proof
		return nil
	}
}

func WithTransferFCTx(ttx *types.TransactionRecord) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransfer = ttx
		return nil
	}
}
