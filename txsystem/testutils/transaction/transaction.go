package transaction

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

const (
	defaultNetworkID types.NetworkID = 5
	defaultSystemID  types.SystemID  = 1
)

func defaultTx() *types.TransactionOrder {
	payload := types.Payload{
		NetworkID:      defaultNetworkID,
		SystemID:       defaultSystemID,
		Type:           22,
		UnitID:         test.RandomBytes(33),
		ClientMetadata: defaultClientMetadata(),
	}
	return &types.TransactionOrder{Payload: payload}
}

func defaultClientMetadata() *types.ClientMetadata {
	return &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2}
}

type Option func(*types.TransactionOrder) error

func WithNetworkID(id types.NetworkID) Option {
	return func(tx *types.TransactionOrder) error {
		tx.NetworkID = id
		return nil
	}
}

func WithSystemID(id types.SystemID) Option {
	return func(tx *types.TransactionOrder) error {
		tx.SystemID = id
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
		authProofCBOR, err := types.Cbor.Marshal(authProof)
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

func WithUnlockProof(unlockProof []byte) Option {
	return func(tx *types.TransactionOrder) error {
		tx.StateUnlock = unlockProof
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
		bytes, err := types.Cbor.Marshal(attr)
		if err != nil {
			return err
		}
		tx.Attributes = bytes
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
	return &types.TransactionRecord{Version: 1,
		TransactionOrder: tx,
		ServerMetadata: &types.ServerMetadata{
			ActualFee:        1,
			TargetUnits:      []types.UnitID{tx.UnitID},
			SuccessIndicator: types.TxStatusSuccessful,
		},
	}
}
