package encoder

import (
	"encoding/binary"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
)

const (
	type_id_bytes  = 1
	type_id_u64    = 2
	type_id_u32    = 3
	type_id_string = 4
	type_id_array  = 5
)

/*
TVEnc is an encoder for serializing data info format which can be parsed
by Alphabill Rust predicate SDK.

Encodes simple data as {type id; data} pairs, to encode more complex data
structures additional "tag" can be assigned to each "data record" (see
EncodeTagged method).
*/
type TVEnc struct {
	buf []byte
	err error
}

func (enc *TVEnc) writeTypeTag(typeID uint8) {
	enc.buf = append(enc.buf, typeID)
}

/*
Bytes returns the serialized representation of the data so far and error
if any happened during encoding. Buffer is not reset!
*/
func (enc *TVEnc) Bytes() ([]byte, error) { return enc.buf, enc.err }

func (enc *TVEnc) setErr(err error) error {
	if enc.err == nil {
		enc.err = err
	}
	return err
}

func (enc *TVEnc) WriteBytes(value []byte) {
	enc.writeTypeTag(type_id_bytes)
	enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(value)))
	if len(value) > 0 {
		enc.buf = append(enc.buf, value...)
	}
}

func (enc *TVEnc) WriteString(value string) {
	enc.writeTypeTag(type_id_string)
	enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(value)))
	if len(value) > 0 {
		enc.buf = append(enc.buf, value...)
	}
}

func (enc *TVEnc) WriteUInt64(value uint64) {
	enc.writeTypeTag(type_id_u64)
	enc.buf = binary.LittleEndian.AppendUint64(enc.buf, value)
}

func (enc *TVEnc) WriteUInt32(value uint32) {
	enc.writeTypeTag(type_id_u32)
	enc.buf = binary.LittleEndian.AppendUint32(enc.buf, value)
}

func (enc *TVEnc) Encode(item any) error {
	switch it := item.(type) {
	case []any:
		enc.writeTypeTag(type_id_array)
		enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(it)))
		for _, v := range it {
			if err := enc.Encode(v); err != nil {
				return enc.setErr(fmt.Errorf("encoding array item: %w", err))
			}
		}
	case []byte:
		enc.WriteBytes(it)
	case types.UnitID:
		enc.WriteBytes(it)
	case string:
		enc.WriteString(it)
	case uint32:
		enc.WriteUInt32(it)
	case uint64:
		enc.WriteUInt64(it)
	default:
		return enc.setErr(fmt.Errorf("unsupported type: %T", it))
	}
	return nil
}

type Tag = uint8

func (enc *TVEnc) EncodeTagged(tag Tag, item any) error {
	enc.buf = append(enc.buf, tag)
	return enc.Encode(item)
}
