package test

import (
	"math/rand"
	"time"
	"fmt"
)

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	// #nosec G404
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

func RandomString(len int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, len)
	rand.Read(b)
	return fmt.Sprintf("%x", b)[:len]
}
