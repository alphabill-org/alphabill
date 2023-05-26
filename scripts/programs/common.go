package programs

import (
	"encoding/binary"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/program"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func Uint64ToLEBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, i)
	return bytes
}

func defaultTx(id []byte) *txsystem.Transaction {
	return &txsystem.Transaction{
		SystemId:              program.DefaultProgramsSystemIdentifier,
		TransactionAttributes: new(anypb.Any),
		UnitId:                id,
		OwnerProof:            nil,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: 10},
	}
}

type Option func(*txsystem.Transaction) error

func WithAttributes(attr proto.Message) Option {
	return func(tx *txsystem.Transaction) error {
		return tx.TransactionAttributes.MarshalFrom(attr)
	}
}

func WithClientMetadata(m *txsystem.ClientMetadata) Option {
	return func(tx *txsystem.Transaction) error {
		tx.ClientMetadata = m
		return nil
	}
}

func NewProgramTransaction(id []byte, options ...Option) (*txsystem.Transaction, error) {
	tx := defaultTx(id)
	for _, o := range options {
		if err := o(tx); err != nil {
			return nil, err
		}
	}
	return tx, nil
}
