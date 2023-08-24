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

var (
	unitID               = types.NewUnitID(33, nil, []byte{1}, []byte{0xff}) // TODO: should be a parameter from a partition
	systemID             = []byte{0, 0, 0, 0}
	targetUnitBacklink   = []byte{3}
	backlink             = []byte{4}
	amount               = uint64(50)
	maxFee               = uint64(2)
	earliestAdditionTime = uint64(0)
	latestAdditionTime   = uint64(10)
)

func NewAddFC(t *testing.T, signer abcrypto.Signer, attr *transactions.AddFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewAddFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unitID),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

type AddFeeCreditOption func(*transactions.AddFeeCreditAttributes) AddFeeCreditOption

func NewAddFCAttr(t *testing.T, signer abcrypto.Signer, opts ...AddFeeCreditOption) *transactions.AddFeeCreditAttributes {
	defaultFCTx := &transactions.AddFeeCreditAttributes{}
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
	return func(tx *transactions.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditOwnerCondition = ownerCondition
		return nil
	}
}

func WithTransferFCProof(proof *types.TxProof) AddFeeCreditOption {
	return func(tx *transactions.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransferProof = proof
		return nil
	}
}

func WithTransferFCTx(ttx *types.TransactionRecord) AddFeeCreditOption {
	return func(tx *transactions.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransfer = ttx
		return nil
	}
}
