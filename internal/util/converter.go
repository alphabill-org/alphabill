package util

import "encoding/binary"

func Uint64ToBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, i)
	return bytes
}

func BytesToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func Uint32ToBytes(i uint32) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint32(bytes, i)
	return bytes
}

func BytesToUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
