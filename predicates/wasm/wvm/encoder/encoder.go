package encoder

import (
	"encoding/binary"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
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

/*
setErr stores the first non-nil error sent as argument and returns it when
Bytes() is called - this allows code where each individual Encode() call
doesn't have to be checked for error, we do the error check when reading
the result of whole encoding operation.

It also returns the argument so the method can be used like

	return enc.setErr(fmt.Errorf(...))
*/
func (enc *TVEnc) setErr(err error) error {
	if enc.err == nil {
		enc.err = err
	}
	return err
}

func (enc *TVEnc) WriteBytes(value []byte) {
	enc.writeTypeTag(type_id_bytes)
	enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(value))) /* #nosec G115 its unlikely that provided input slice length exceeds uint32 max value */
	if len(value) > 0 {
		enc.buf = append(enc.buf, value...)
	}
}

func (enc *TVEnc) WriteString(value string) {
	enc.writeTypeTag(type_id_string)
	enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(value))) /* #nosec G115 its unlikely that provided input string length exceeds uint32 max value */
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

func (enc *TVEnc) Encode(item any) {
	switch it := item.(type) {
	case []any:
		enc.writeTypeTag(type_id_array)
		enc.buf = binary.LittleEndian.AppendUint32(enc.buf, uint32(len(it))) /* #nosec G115 its unlikely that it length exceeds uint32 max value */
		for _, v := range it {
			enc.Encode(v)
		}
	case []byte:
		enc.WriteBytes(it)
	case hex.Bytes:
		enc.WriteBytes(it)
	case types.RawCBOR:
		enc.WriteBytes(it)
	case types.UnitID:
		enc.WriteBytes(it)
	case string:
		enc.WriteString(it)
	case uint16:
		enc.WriteUInt32(uint32(it))
	case uint32:
		enc.WriteUInt32(it)
	case uint64:
		enc.WriteUInt64(it)
	default:
		_ = enc.setErr(fmt.Errorf("unsupported type: %T", it))
	}
}

type Tag = uint8

func (enc *TVEnc) EncodeTagged(tag Tag, item any) {
	enc.buf = append(enc.buf, tag)
	enc.Encode(item)
}
