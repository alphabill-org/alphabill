package test

import (
	"crypto/rand"
	"fmt"
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
