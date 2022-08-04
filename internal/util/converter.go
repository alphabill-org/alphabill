package util

import "encoding/binary"

func Uint64ToBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, i)
	return bytes
}

func BytesToUint64(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
}
