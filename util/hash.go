package util

import (
	"crypto"
	"crypto/sha256"
	"crypto/sha512"
)

var (
	Zero256 = make([]byte, 32)
	Zero512 = make([]byte, 64)
)

// Sum256 returns the SHA256 checksum of the data.
// Returns zero hash if data is empty or nil
func Sum256(data []byte) []byte {
	// return zero hash in case data is either empty or missing
	if len(data) == 0 {
		return Zero256
	}
	hsh := sha256.Sum256(data)
	return hsh[:]
}

// Sum512 returns the SHA512 checksum of the data.
// Returns zero hash if data is empty or nil
func Sum512(data []byte) []byte {
	// return zero hash in case data is either empty or missing
	if len(data) == 0 {
		return Zero512
	}
	hsh := sha512.Sum512(data)
	return hsh[:]
}

// Sum hashes together arbitrary data units
// NB! Does not return zero hash if no data is provided!
func Sum(hashAlgorithm crypto.Hash, hashes ...[]byte) []byte {
	hasher := hashAlgorithm.New()
	for _, hash := range hashes {
		hasher.Write(hash)
	}
	return hasher.Sum(nil)
}
