package types

import (
	"errors"

	"github.com/fxamacker/cbor/v2"
)

var cborNil = []byte{0xf6}

type (
	TransactionOrder struct {
		_          struct{} `cbor:",toarray"`
		Payload    *Payload
		OwnerProof []byte
		FeeProof   []byte
	}

	Payload struct {
		_              struct{} `cbor:",toarray"`
		SystemID       SystemID
		Type           string
		UnitID         UnitID
		Attributes     RawCBOR
		ClientMetadata *ClientMetadata
	}

	ClientMetadata struct {
		_                 struct{} `cbor:",toarray"`
		Timeout           int64
		MaxTransactionFee int64
		FeeCreditRecordID []byte
	}

	RawCBOR []byte
)

func (t *TransactionOrder) PayloadBytes() ([]byte, error) {
	return cbor.Marshal(t.Payload)
}

func (t *TransactionOrder) UnmarshalAttributes(v any) error {
	if t == nil {
		return errors.New("transaction order is nil")
	}
	return t.Payload.UnmarshalAttributes(v)
}

func (p *Payload) UnmarshalAttributes(v any) error {
	if p == nil {
		return errors.New("payload is nil")
	}
	return cbor.Unmarshal(p.Attributes, v)
}

// MarshalCBOR returns r or CBOR nil if r is nil.
func (r RawCBOR) MarshalCBOR() ([]byte, error) {
	if len(r) == 0 {
		return cborNil, nil
	}
	return r, nil
}

// UnmarshalCBOR creates a copy of data and saves to *r.
func (r *RawCBOR) UnmarshalCBOR(data []byte) error {
	if r == nil {
		return errors.New("UnmarshalCBOR on nil pointer")
	}
	*r = make([]byte, len(data))
	copy(*r, data)
	return nil
}
