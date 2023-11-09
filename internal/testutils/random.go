package test

import (
	"crypto/rand"
	"fmt"
	mrand "math/rand"
)

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

func RandomString(len int) string {
	b := RandomBytes(len)
	return fmt.Sprintf("%x", b)[:len]
}

func RandomUint64() uint64 {
	return mrand.Uint64()
}
