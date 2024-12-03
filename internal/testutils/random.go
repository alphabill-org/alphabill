package test

import (
	"crypto/rand"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/util"
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
	b := RandomBytes(len/2 + 1)
	return fmt.Sprintf("%x", b)[:len]
}

func RandomUint32() uint32 {
	bytes := make([]byte, 4)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return util.BytesToUint32(bytes)
}
