package types

import (
	"crypto"

	"github.com/fxamacker/cbor/v2"
)

const (
	// TxStatusFailed is the status code of a transaction if execution failed.
	TxStatusFailed = uint64(0)

	// TxStatusSuccessful is the status code of a transaction if execution succeeded.
	TxStatusSuccessful = uint64(1)
)

type (

	// TransactionRecord is a transaction order with "server-side" metadata added to it. TransactionRecord is a structure
	// that is added to the block.
	TransactionRecord struct {
		_                struct{} `cbor:",toarray"`
		TransactionOrder *TransactionOrder
		ServerMetadata   *ServerMetadata
	}

	ServerMetadata struct {
		_                 struct{} `cbor:",toarray"`
		ActualFee         uint64
		TargetUnits       []UnitID
		SuccessIndicator  uint64
		ProcessingDetails []byte
	}
)

func (t *TransactionRecord) Hash(algorithm crypto.Hash) []byte {
	bytes, err := t.Bytes()
	if err != nil {
		// TODO
		panic(err)
	}
	hasher := algorithm.New()
	hasher.Write(bytes)
	return hasher.Sum(nil)
}

func (t *TransactionRecord) Bytes() ([]byte, error) {
	return cbor.Marshal(t)
}

func (sm *ServerMetadata) GetActualFee() uint64 {
	if sm == nil {
		return 0
	}
	return sm.ActualFee
}
