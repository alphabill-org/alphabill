package types

import (
	"crypto"
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
		Timeout           uint64
		MaxTransactionFee uint64
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

func (t *TransactionOrder) UnitID() []byte {
	if t.Payload == nil {
		return nil
	}
	return t.Payload.UnitID
}

func (t *TransactionOrder) SystemID() []byte {
	if t.Payload == nil {
		return nil
	}
	return t.Payload.SystemID
}

func (t *TransactionOrder) Timeout() uint64 {
	if t.Payload == nil || t.Payload.ClientMetadata == nil {
		return 0
	}
	return t.Payload.ClientMetadata.Timeout
}

func (t *TransactionOrder) PayloadType() string {
	if t.Payload == nil {
		return ""
	}
	return t.Payload.Type
}

func (t *TransactionOrder) GetClientFeeCreditRecordID() []byte {
	if t.Payload == nil || t.Payload.ClientMetadata == nil {
		return nil
	}
	return t.Payload.ClientMetadata.FeeCreditRecordID
}

func (t *TransactionOrder) Hash(algorithm crypto.Hash) []byte {
	hasher := algorithm.New()
	bytes, err := cbor.Marshal(t)
	if err != nil {
		//TODO
		panic(err)
	}
	hasher.Write(bytes)
	return hasher.Sum(nil)
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
