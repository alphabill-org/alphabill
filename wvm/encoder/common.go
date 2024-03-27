package encoder

import (
	"fmt"
	"reflect"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
)

// returns tx order attributes encoded to WASM representation
type TxAttributesEncoder func(txo *types.TransactionOrder) ([]byte, error)

type UnitDataEncoder func(data state.UnitData) ([]byte, error)

// tx attribute encoder ID
type AttrEncID struct {
	TxSys types.SystemID
	Attr  string // tx attributes type id (payload type name)
	// version of the encoding. makes sense when SDK can request version / supports multiple versions?
	// when using the SDK version to determine the response encoding then it is probably
	// more flexible/easier is to send version as a param to the encoder func - then when nothing
	// changes between SDK versions don't have to repeat same encoder for different SDK versions?
	//Ver   int
}

/*
TXSystemEncoder is "generic" tx system encoder, parts specific to given tx system (which wants to
use it with Wazero WASM predicates) must be added using Register*Encoder methods.
*/
type TXSystemEncoder struct {
	attrEnc map[AttrEncID]TxAttributesEncoder
	udEnc   map[reflect.Type]UnitDataEncoder
}

/*
Encode serializes well known types (not tx system specific) to WASM representation.

getHandle - add type id param and encode it into handle? ie CBOR, BO, []byte,...?
*/
func (enc TXSystemEncoder) Encode(obj any, getHandle func(obj any) uint64) ([]byte, error) {
	switch t := obj.(type) {
	case *types.TransactionOrder:
		return enc.txOrder(t)
	case *types.TransactionRecord:
		return enc.txRecord(t, getHandle)
	case []byte:
		return t, nil
	}
	return nil, fmt.Errorf("no encoder for %T", obj)
}

func (TXSystemEncoder) txRecord(txo *types.TransactionRecord, getHandle func(obj any) uint64) ([]byte, error) {
	var buf WasmEnc
	buf.WriteTypeVer(type_id_tx_record, 1)
	buf.WriteUInt64(getHandle(txo.TransactionOrder))
	return buf, nil
}

func (TXSystemEncoder) txOrder(txo *types.TransactionOrder) ([]byte, error) {
	buf := make(WasmEnc, 0, (6*4)+len(txo.Payload.Type)+len(txo.Payload.UnitID)+len(txo.OwnerProof)+len(txo.FeeProof))
	buf.WriteTypeVer(type_id_tx_order, 1)
	buf.WriteUInt32(uint32(txo.SystemID()))
	buf.WriteString(txo.Payload.Type)
	buf.WriteBytes(txo.Payload.UnitID)
	return buf, nil
}

func (enc TXSystemEncoder) TxAttributes(txo *types.TransactionOrder) ([]byte, error) {
	fn, ok := enc.attrEnc[AttrEncID{TxSys: txo.SystemID(), Attr: txo.PayloadType()}]
	if !ok {
		return nil, fmt.Errorf("serializing to bytes is not implemented for tx system %d %q attributes", txo.Payload.SystemID, txo.PayloadType())
	}
	return fn(txo)
}

func (enc TXSystemEncoder) UnitData(unit *state.Unit) ([]byte, error) {
	data := unit.Data()
	fn, ok := enc.udEnc[reflect.TypeOf(data)]
	if !ok {
		return nil, fmt.Errorf("serializing to bytes is not implemented for unit data type %T", data)
	}
	return fn(data)
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
func (enc *TXSystemEncoder) RegisterAttrEncoder(id AttrEncID, encoder TxAttributesEncoder) error {
	if enc.attrEnc == nil {
		enc.attrEnc = make(map[AttrEncID]TxAttributesEncoder)
	}
	if _, ok := enc.attrEnc[id]; ok {
		return fmt.Errorf("tx attribute encoder for %v is already registered", id)
	}
	enc.attrEnc[id] = encoder
	return nil
}

/*
reg is a func which attempts to register "all" encoders and for each "filter" is
executed and only these for which filter returned true actual registration is
attempted.
*/
func (enc *TXSystemEncoder) RegisterTxAttributeEncoders(reg func(func(id AttrEncID, enc TxAttributesEncoder) error) error, filter func(AttrEncID) bool) error {
	return reg(func(id AttrEncID, encoder TxAttributesEncoder) error {
		if !filter(id) {
			return nil
		}
		return enc.RegisterAttrEncoder(id, encoder)
	})
}

const (
	type_id_tx_order  = 1
	type_id_tx_record = 8
	type_id_tx_proof  = 9
)
