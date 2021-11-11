package canonicalizer

import (
	"encoding/binary"
)

func Uint64ToBytes(val uint64) []byte {
	bin := make([]byte, 8)
	binary.BigEndian.PutUint64(bin, val)
	return bin
}

func BytesToUint64(val []byte) uint64 {
	return binary.BigEndian.Uint64(val)
}
