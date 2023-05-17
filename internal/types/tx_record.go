package types

import (
	"crypto"

	"github.com/fxamacker/cbor/v2"
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
		_           struct{} `cbor:",toarray"`
		ActualFee   uint64
		TargetUnits []UnitID
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
