package encoder

import (
	"fmt"
	"reflect"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/state"
)

type (
	// returns tx order attributes encoded to WASM representation
	TxAttributesEncoder func(txo *types.TransactionOrder, ver uint32) ([]byte, error)

	// returns unit data encoded to WASM representation
	UnitDataEncoder func(data types.UnitData, ver uint32) ([]byte, error)

	// returns authorization proof (aka OwnerProof) of the txo.
	AuthProof func(txo *types.TransactionOrder) ([]byte, error)
)

type PartitionTxType struct {
	Partition types.PartitionID
	TxType    uint16 // tx type id
}

/*
TXSystemEncoder is "generic" tx system encoder, parts specific to given tx system (which
wants to use it with Wazero WASM predicates) must be added using Register*Encoder methods.
*/
type TXSystemEncoder struct {
	attrEnc map[PartitionTxType]TxAttributesEncoder
	udEnc   map[reflect.Type]UnitDataEncoder
	auth    map[PartitionTxType]AuthProof
}

func New(f ...any) (TXSystemEncoder, error) {
	enc := TXSystemEncoder{}
	for x, v := range f {
		switch rf := v.(type) {
		case func(func(id PartitionTxType, enc TxAttributesEncoder) error) error:
			if err := rf(enc.RegisterAttrEncoder); err != nil {
				return enc, fmt.Errorf("registering attribute encoder [%d]: %w", x, err)
			}
		case func(func(ud any, encoder UnitDataEncoder) error) error:
			if err := rf(enc.RegisterUnitDataEncoder); err != nil {
				return enc, fmt.Errorf("registering unit-data encoder [%d]: %w", x, err)
			}
		case func(reg func(id PartitionTxType, enc AuthProof) error) error:
			if err := rf(enc.RegisterAuthProof); err != nil {
				return enc, fmt.Errorf("registering auth proof handlers [%d]: %w", x, err)
			}
		default:
			return enc, fmt.Errorf("unsupported registration function type [%d]: %T", x, v)
		}
	}
	return enc, nil
}

/*
Encode serializes well known types (not tx system specific) to representation WASM
predicate SDK can load.

  - obj: data to serialize, must be of "well known type";
  - ver: version of the encoding/object the predicate expects;
  - getHandle: callback to register variable in the execution context, returns handle
    of the new variable. Ie instead of "flattening" sub-object it can be registered and
    it's handle returned as part of response allowing predicate to load the sub-object
    with next call.
*/
func (enc TXSystemEncoder) Encode(obj any, ver uint32, getHandle func(obj any) uint64) ([]byte, error) {
	switch t := obj.(type) {
	case *types.TransactionOrder:
		return enc.txOrder(t, ver)
	case *types.TransactionRecord:
		return enc.txRecord(t, ver, getHandle)
	case []byte:
		return t, nil
	case types.RawCBOR:
		return t, nil
	}
	return nil, fmt.Errorf("no encoder for %T", obj)
}

func (TXSystemEncoder) txRecord(txo *types.TransactionRecord, _ uint32, getHandle func(obj any) uint64) ([]byte, error) {
	var buf TVEnc
	buf.EncodeTagged(1, getHandle(txo.TransactionOrder))
	return buf.Bytes()
}

func (TXSystemEncoder) txOrder(txo *types.TransactionOrder, _ uint32) ([]byte, error) {
	var buf TVEnc
	buf.EncodeTagged(1, uint32(txo.PartitionID))
	buf.EncodeTagged(2, txo.UnitID)
	buf.EncodeTagged(3, txo.Type)
	buf.EncodeTagged(4, txo.ReferenceNumber())
	return buf.Bytes()
}

func (enc TXSystemEncoder) TxAttributes(txo *types.TransactionOrder, ver uint32) ([]byte, error) {
	encoder, ok := enc.attrEnc[PartitionTxType{txo.PartitionID, txo.Type}]
	if !ok {
		return nil, fmt.Errorf("serializing to bytes is not implemented for transaction system %d %q attributes", txo.PartitionID, txo.Type)
	}
	return encoder(txo, ver)
}

func (enc TXSystemEncoder) UnitData(unit state.Unit, ver uint32) ([]byte, error) {
	data := unit.Data()
	encoder, ok := enc.udEnc[reflect.TypeOf(data)]
	if !ok {
		return nil, fmt.Errorf("serializing to bytes is not implemented for unit data type %T", data)
	}
	return encoder(data, ver)
}

func (enc TXSystemEncoder) AuthProof(txo *types.TransactionOrder) ([]byte, error) {
	f, ok := enc.auth[PartitionTxType{txo.PartitionID, txo.Type}]
	if !ok {
		return nil, fmt.Errorf("auth proof handler for partition %s tx type %d not registered", txo.PartitionID, txo.Type)
	}
	return f(txo)
}

func (enc *TXSystemEncoder) RegisterAuthProof(id PartitionTxType, handler AuthProof) error {
	if enc.auth == nil {
		enc.auth = make(map[PartitionTxType]AuthProof)
	}
	if _, ok := enc.auth[id]; ok {
		return fmt.Errorf("tx auth proof handler for %v is already registered", id)
	}
	enc.auth[id] = handler
	return nil
}

func (enc *TXSystemEncoder) RegisterUnitDataEncoder(ud any, encoder UnitDataEncoder) error {
	if enc.udEnc == nil {
		enc.udEnc = make(map[reflect.Type]UnitDataEncoder)
	}
	rt := reflect.TypeOf(ud)
	if _, ok := enc.udEnc[rt]; ok {
		return fmt.Errorf("unit data encoder for %T is already registered", ud)
	}
	enc.udEnc[rt] = encoder
	return nil
}

/*
RegisterAttrEncoder registers tx attribute encoder.
*/
func (enc *TXSystemEncoder) RegisterAttrEncoder(id PartitionTxType, encoder TxAttributesEncoder) error {
	if enc.attrEnc == nil {
		enc.attrEnc = make(map[PartitionTxType]TxAttributesEncoder)
	}
	if _, ok := enc.attrEnc[id]; ok {
		return fmt.Errorf("tx attribute encoder for %v is already registered", id)
	}
	enc.attrEnc[id] = encoder
	return nil
}

/*
RegisterTxAttributeEncoders is like RegisterAttrEncoder but allows to filter
out undesired encoders.
  - reg: func which attempts to register "all" encoders but for each "filter" is
    executed and only these for which filter returned true actual registration is
    attempted.
*/
func (enc *TXSystemEncoder) RegisterTxAttributeEncoders(reg func(func(id PartitionTxType, enc TxAttributesEncoder) error) error, filter func(PartitionTxType) bool) error {
	return reg(func(id PartitionTxType, encoder TxAttributesEncoder) error {
		if !filter(id) {
			return nil
		}
		return enc.RegisterAttrEncoder(id, encoder)
	})
}
