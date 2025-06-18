package transaction

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

const (
	defaultNetworkID   types.NetworkID   = 5
	defaultPartitionID types.PartitionID = 1
)

func defaultTx() *types.TransactionOrder {
	payload := types.Payload{
		NetworkID:      defaultNetworkID,
		PartitionID:    defaultPartitionID,
		Type:           50,
		UnitID:         test.RandomBytes(33),
		ClientMetadata: defaultClientMetadata(),
	}
	return &types.TransactionOrder{Version: 1, Payload: payload}
}

func defaultClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2}
}

type Option func(*types.TransactionOrder) error

func WithPartition(pdr *types.PartitionDescriptionRecord) Option {
	return func(tx *types.TransactionOrder) error {
		tx.NetworkID = pdr.NetworkID
		tx.PartitionID = pdr.PartitionID
		return nil
	}
}

func WithNetworkID(id types.NetworkID) Option {
	return func(tx *types.TransactionOrder) error {
		tx.NetworkID = id
		return nil
	}
}

func WithPartitionID(id types.PartitionID) Option {
	return func(tx *types.TransactionOrder) error {
		tx.PartitionID = id
		return nil
	}
}

func WithUnitID(id []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.UnitID = id
		return nil
	}
}

func WithTransactionType(t uint16) Option {
	return func(tx *types.TransactionOrder) error {
		tx.Type = t
		return nil
	}
}

func WithAuthProof(authProof any) Option {
	return func(tx *types.TransactionOrder) error {
		authProofCBOR, err := cbor.Marshal(authProof)
		if err != nil {
			return err
		}
		tx.AuthProof = authProofCBOR
		return nil
	}
}

func WithFeeProof(feeProof []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.FeeProof = feeProof
		return nil
	}
}

func WithStateUnlock(stateUnlock []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.StateUnlock = stateUnlock
		return nil
	}
}

func WithClientMetadata(m *types.ClientMetadata) Option {
	return func(tx *types.TransactionOrder) error {
		tx.ClientMetadata = m
		return nil
	}
}

func WithStateLock(lock *types.StateLock) Option {
	return func(tx *types.TransactionOrder) error {
		tx.StateLock = lock
		return nil
	}
}

func WithAttributes(attr any) Option {
	return func(tx *types.TransactionOrder) error {
		bytes, err := cbor.Marshal(attr)
		if err != nil {
			return err
		}
		tx.Attributes = bytes
		return nil
	}
}

func NewTransactionOrderBytes(t *testing.T, options ...Option) cbor.TaggedCBOR {
	tx := NewTransactionOrder(t, options...)
	txBytes, err := tx.MarshalCBOR()
	require.NoError(t, err)
	return txBytes
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
	txBytes, err := tx.MarshalCBOR()
	require.NoError(t, err)
	return &types.TransactionRecord{
		Version:          1,
		TransactionOrder: txBytes,
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        1,
			TargetUnits:      []types.UnitID{tx.UnitID},
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
}

func TxoToBytes(t *testing.T, tx *types.TransactionOrder) cbor.TaggedCBOR {
	txBytes, err := tx.MarshalCBOR()
	require.NoError(t, err)
	return txBytes
}

func TxoFromBytes(t *testing.T, txBytes cbor.TaggedCBOR) *types.TransactionOrder {
	tx := &types.TransactionOrder{Version: 1}
	require.NoError(t, tx.UnmarshalCBOR(txBytes))
	return tx
}

type TxoV1Fetcher interface {
	GetTransactionOrderV1() (*types.TransactionOrder, error)
}

func FetchTxoV1(t *testing.T, tx TxoV1Fetcher) *types.TransactionOrder {
	txoV1, err := tx.GetTransactionOrderV1()
	require.NoError(t, err)
	return txoV1
}
