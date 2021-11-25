package test

import "math/rand"

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	// #nosec 404 - using math/rand, test code
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}
