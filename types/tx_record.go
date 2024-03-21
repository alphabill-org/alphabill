package types

import (
	"crypto"
	"errors"

	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
)

const (
	// TxStatusFailed is the status code of a transaction if execution failed.
	TxStatusFailed TxStatus = 0
	// TxStatusSuccessful is the status code of a transaction if execution succeeded.
	TxStatusSuccessful TxStatus = 1
)

type (
	TxStatus uint64

	// TransactionRecord is a transaction order with "server-side" metadata added to it. TransactionRecord is a structure
	// that is added to the block.
	TransactionRecord struct {
		_                struct{} `cbor:",toarray"`
		TransactionOrder *TransactionOrder
		ServerMetadata   *ServerMetadata
	}

	ServerMetadata struct {
		_                 struct{} `cbor:",toarray"`
		ActualFee         fct.Fee
		TargetUnits       []UnitID
		SuccessIndicator  TxStatus
		ProcessingDetails RawCBOR
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
	return Cbor.Marshal(t)
}

func (t *TransactionRecord) UnmarshalProcessingDetails(v any) error {
	if t == nil {
		return errors.New("transaction record is nil")
	}
	return t.ServerMetadata.UnmarshalDetails(v)
}

func (t *TransactionRecord) GetActualFee() fct.Fee {
	if t == nil {
		return 0
	}
	return t.ServerMetadata.GetActualFee()
}

func (sm *ServerMetadata) GetActualFee() fct.Fee {
	if sm == nil {
		return 0
	}
	return sm.ActualFee
}

func (sm *ServerMetadata) UnmarshalDetails(v any) error {
	if sm == nil {
		return errors.New("server metadata is nil")
	}
	return Cbor.Unmarshal(sm.ProcessingDetails, v)
}
