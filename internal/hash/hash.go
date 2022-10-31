package hash

import (
	"crypto"
	"crypto/sha256"
	"crypto/sha512"
)

var Zero256 = make([]byte, 32)

// Sum256 returns the SHA256 checksum of the data using the MessageHash augmented hashing.
func Sum256(data []byte) []byte {
	// return zero hash in case data is either empty or missing
	if len(data) == 0 {
		return Zero256
	}
	hsh := sha256.Sum256(data)
	return hsh[:]
}

// Sum512 returns the SHA512 checksum of the data.
func Sum512(data []byte) []byte {
	hsh := sha512.Sum512(data)
	return hsh[:]
}

// Sum hashes together arbitary data units
func Sum(hashAlgorithm crypto.Hash, hashes ...[]byte) []byte {
	hasher := hashAlgorithm.New()
	for _, hash := range hashes {
		hasher.Write(hash)
	}
	return hasher.Sum(nil)
}
