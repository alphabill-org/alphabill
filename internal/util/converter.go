package util

import (
	"encoding/binary"

	"github.com/holiman/uint256"
)

func Uint64ToBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.BigEndian.PutUint64(bytes, i)
	return bytes
}

func BytesToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}

func Uint32ToBytes(i uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, i)
	return bytes
}

func Uint256ToBytes(i *uint256.Int) []byte {
	b := i.Bytes32()
	return b[:]
}

func BytesToUint256(b []byte) *uint256.Int {
	i := uint256.NewInt(0)
	return i.SetBytes(b)
}
