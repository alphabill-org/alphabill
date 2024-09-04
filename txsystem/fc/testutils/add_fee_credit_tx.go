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
	systemID           types.SystemID = 1
	targetUnitCounter                 = uint64(3)
	targetCounter                     = uint64(4)
	counter                           = uint64(4)
	amount                            = uint64(50)
	maxFee                            = uint64(2)
	latestAdditionTime                = uint64(10)
)

func NewAddFC(t *testing.T, signer abcrypto.Signer, attr *fc.AddFeeCreditAttributes, opts ...testtransaction.Option) *types.TransactionOrder {
	if attr == nil {
		attr = NewAddFCAttr(t, signer)
	}
	tx := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(NewFeeCreditRecordID(t, signer)),
		testtransaction.WithAttributes(attr),
		testtransaction.WithPayloadType(fc.PayloadTypeAddFeeCredit),
		testtransaction.WithAuthProof(fc.AddFeeCreditAuthProof{}),
	)
	for _, opt := range opts {
		require.NoError(t, opt(tx))
	}
	return tx
}

type AddFeeCreditOption func(*fc.AddFeeCreditAttributes) AddFeeCreditOption

func NewAddFCAttr(t *testing.T, signer abcrypto.Signer, opts ...AddFeeCreditOption) *fc.AddFeeCreditAttributes {
	attr := &fc.AddFeeCreditAttributes{}
	for _, opt := range opts {
		opt(attr)
	}
	if attr.FeeCreditTransfer == nil {
		attr.FeeCreditTransfer = &types.TransactionRecord{
			TransactionOrder: NewTransferFC(t, signer, nil),
			ServerMetadata:   &types.ServerMetadata{SuccessIndicator: types.TxStatusSuccessful},
		}
	}
	if attr.FeeCreditTransferProof == nil {
		attr.FeeCreditTransferProof = testblock.CreateProof(t, attr.FeeCreditTransfer, signer)
	}
	if attr.FeeCreditOwnerPredicate == nil {
		attr.FeeCreditOwnerPredicate = NewP2pkhFeeAuthPredicate(t, signer)
	}
	return attr
}

func WithFeeCreditOwnerPredicate(ownerPredicate []byte) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditOwnerPredicate = ownerPredicate
		return nil
	}
}

func WithTransferFCProof(proof *types.TxProof) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransferProof = proof
		return nil
	}
}

func WithTransferFCRecord(ttx *types.TransactionRecord) AddFeeCreditOption {
	return func(tx *fc.AddFeeCreditAttributes) AddFeeCreditOption {
		tx.FeeCreditTransfer = ttx
		return nil
	}
}
