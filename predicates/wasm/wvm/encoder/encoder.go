package encoder

import "encoding/binary"

/*
WasmEnc is a helper type for serializing data into representation
which can be consumed by the Alphabill-predicates Rust SDK.
*/
type WasmEnc []byte

func (buf *WasmEnc) WriteTypeVer(typID, ver uint16) {
	*buf = binary.LittleEndian.AppendUint32(*buf, (uint32(ver)<<16)+uint32(typID))
}

func (buf *WasmEnc) WriteBytes(value []byte) {
	*buf = binary.LittleEndian.AppendUint32(*buf, uint32(len(value)))
	*buf = append(*buf, value...)
}

func (buf *WasmEnc) WriteString(value string) {
	buf.WriteBytes([]byte(value))
}

func (buf *WasmEnc) WriteUInt64(value uint64) {
	*buf = binary.LittleEndian.AppendUint64(*buf, value)
}

func (buf *WasmEnc) WriteUInt32(value uint32) {
	*buf = binary.LittleEndian.AppendUint32(*buf, value)
}
