package testtransaction

import (
	"testing"

	"github.com/alphabill-org/alphabill/testutils"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var defaultSystemID = []byte{0, 0, 0, 0}

func defaultTx() *types.TransactionOrder {
	payload := &types.Payload{
		SystemID:       defaultSystemID,
		Type:           "test",
		UnitID:         test.RandomBytes(33),
		ClientMetadata: defaultClientMetadata(),
	}

	return &types.TransactionOrder{
		Payload:    payload,
		OwnerProof: nil,
	}
}

func defaultClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2}
}

type Option func(*types.TransactionOrder) error

func WithSystemID(id types.SystemID) Option {
	return func(tx *types.TransactionOrder) error {
		tx.Payload.SystemID = id
		return nil
	}
}

func WithUnitId(id []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.Payload.UnitID = id
		return nil
	}
}

func WithPayloadType(t string) Option {
	return func(tx *types.TransactionOrder) error {
		tx.Payload.Type = t
		return nil
	}
}

func WithOwnerProof(ownerProof []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.OwnerProof = ownerProof
		return nil
	}
}

func WithFeeProof(feeProof []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.FeeProof = feeProof
		return nil
	}
}

func WithClientMetadata(m *types.ClientMetadata) Option {
	return func(tx *types.TransactionOrder) error {
		tx.Payload.ClientMetadata = m
		return nil
	}
}

func WithAttributes(attr any) Option {
	return func(tx *types.TransactionOrder) error {
		bytes, err := cbor.Marshal(attr)
		if err != nil {
			return err
		}
		tx.Payload.Attributes = bytes
		return nil
	}
}

func NewTransactionOrder(t *testing.T, options ...Option) *types.TransactionOrder {
	tx := defaultTx()
	for _, o := range options {
		require.NoError(t, o(tx))
	}
	return tx
}

func NewTransactionRecord(t *testing.T, options ...Option) *types.TransactionRecord {
	tx := defaultTx()
	for _, o := range options {
		require.NoError(t, o(tx))
	}
	return &types.TransactionRecord{
		TransactionOrder: tx,
		ServerMetadata: &types.ServerMetadata{
			ActualFee:   1,
			TargetUnits: []types.UnitID{tx.UnitID()},
		},
	}
}
