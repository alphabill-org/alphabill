package testtransaction

import (
	"math/rand"
	"testing"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var moneySystemID = []byte{0, 0, 0, 0}

func defaultTx() *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              moneySystemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                RandomBytes(32),
		Timeout:               10,
		OwnerProof:            RandomBytes(3),
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: 10, MaxFee: 2},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}
}

type Option func(*txsystem.Transaction) error

type ConvertTx func(*txsystem.Transaction) (txsystem.GenericTransaction, error)

func WithSystemID(id []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.SystemId = id
		return nil
	}
}

func WithUnitId(id []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.UnitId = id
		return nil
	}
}

func WithTimeout(timeout uint64) Option {
	return func(tx *txsystem.Transaction) error {
		tx.Timeout = timeout
		return nil
	}
}

func WithOwnerProof(ownerProof []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.OwnerProof = ownerProof
		return nil
	}
}

func WithFeeProof(feeProof []byte) Option {
	return func(tx *txsystem.Transaction) error {
		tx.FeeProof = feeProof
		return nil
	}
}

func WithClientMetadata(m *txsystem.ClientMetadata) Option {
	return func(tx *txsystem.Transaction) error {
		tx.ClientMetadata = m
		return nil
	}
}

func WithServerMetadata(m *txsystem.ServerMetadata) Option {
	return func(tx *txsystem.Transaction) error {
		tx.ServerMetadata = m
		return nil
	}
}

func WithAttributes(attr proto.Message) Option {
	return func(tx *txsystem.Transaction) error {
		return tx.TransactionAttributes.MarshalFrom(attr)
	}
}

func NewTransaction(t *testing.T, options ...Option) *txsystem.Transaction {
	tx := defaultTx()
	for _, o := range options {
		require.NoError(t, o(tx))
	}
	return tx
}

func NewGenericTransaction(t *testing.T, c ConvertTx, options ...Option) txsystem.GenericTransaction {
	tx := NewTransaction(t, options...)
	genTx, err := c(tx)
	require.NoError(t, err)
	return genTx
}

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	// #nosec G404
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}
