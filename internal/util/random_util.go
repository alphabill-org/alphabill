package util

import (
	"crypto/rand"
	"github.com/holiman/uint256"
)

func RandomBytes(len int) ([]byte, error) {
	bytes := make([]byte, len)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	return bytes, nil
}

func RandomUint256() (*uint256.Int, error) {
	b, err := RandomBytes(32)
	if err != nil {
		return nil, err
	}
	return uint256.NewInt(0).SetBytes(b), nil
}
