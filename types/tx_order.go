package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

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

	ProofGenerator func(bytesToSign []byte) (proof []byte, err error)

	SigBytesProvider interface {
		SigBytes() ([]byte, error)
	}
)

func (t *TransactionOrder) PayloadBytes() ([]byte, error) {
	return t.Payload.Bytes()
}

func (t *TransactionOrder) UnmarshalAttributes(v any) error {
	if t == nil {
		return errors.New("transaction order is nil")
	}
	return t.Payload.UnmarshalAttributes(v)
}

func (t *TransactionOrder) UnitID() UnitID {
	if t.Payload == nil {
		return nil
	}
	return t.Payload.UnitID
}

func (t *TransactionOrder) SystemID() SystemID {
	if t.Payload == nil {
		return 0
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

func (t *TransactionOrder) GetClientMaxTxFee() uint64 {
	if t.Payload == nil || t.Payload.ClientMetadata == nil {
		return 0
	}
	return t.Payload.ClientMetadata.MaxTransactionFee
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

/*
SetOwnerProof assigns the bytes returned by the function provided as argument to
the OwnerProof field unless the function (or reading data to be signed by that
function) returned error.
*/
func (t *TransactionOrder) SetOwnerProof(proofer ProofGenerator) error {
	data, err := t.PayloadBytes()
	if err != nil {
		return fmt.Errorf("reading payload bytes to sign: %w", err)
	}
	if t.OwnerProof, err = proofer(data); err != nil {
		return fmt.Errorf("generating owner proof: %w", err)
	}
	return nil
}

/*
SetAttributes serializes "attr" and assigns the result to payload's Attributes field.
The "attr" is expected to be one of the transaction attribute structs but there is
no validation!
The Payload.UnmarshalAttributes can be used to decode the attributes.
*/
func (p *Payload) SetAttributes(attr any) error {
	bytes, err := cbor.Marshal(attr)
	if err != nil {
		return fmt.Errorf("marshaling %T as tx attributes: %w", attr, err)
	}
	p.Attributes = bytes
	return nil
}

func (p *Payload) UnmarshalAttributes(v any) error {
	if p == nil {
		return errors.New("payload is nil")
	}
	return cbor.Unmarshal(p.Attributes, v)
}

func (p *Payload) Bytes() ([]byte, error) {
	return cbor.Marshal(p)
}

// BytesWithAttributeSigBytes TODO: AB-1016 remove this hack
func (p *Payload) BytesWithAttributeSigBytes(attrs SigBytesProvider) ([]byte, error) {
	attrBytes, err := attrs.SigBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attributes: %w", err)
	}
	payload := &Payload{
		SystemID:       p.SystemID,
		Type:           p.Type,
		UnitID:         p.UnitID,
		Attributes:     attrBytes,
		ClientMetadata: p.ClientMetadata,
	}
	return payload.Bytes()
}

// MarshalCBOR returns r or CBOR nil if r is nil.
func (r RawCBOR) MarshalCBOR() ([]byte, error) {
	if len(r) == 0 || bytes.Equal(r, cborNil) {
		return cborNil, nil
	}
	return r, nil
}

// UnmarshalCBOR creates a copy of data and saves to *r.
func (r *RawCBOR) UnmarshalCBOR(data []byte) error {
	if r == nil {
		return errors.New("UnmarshalCBOR on nil pointer")
	}
	if bytes.Equal(data, cborNil) {
		return nil
	}
	*r = make([]byte, len(data))
	copy(*r, data)
	return nil
}
