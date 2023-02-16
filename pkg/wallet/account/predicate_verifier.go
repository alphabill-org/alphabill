package account

import (
	"bytes"

	"github.com/alphabill-org/alphabill/internal/script"
)

// VerifyP2PKHOwner checks if given bearer predicate equals either SHA256 or SHA512 P2PKH predicate.
func VerifyP2PKHOwner(pubkeyHashes *KeyHashes, bp []byte) bool {
	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bp) != 42 && len(bp) != 74 {
		return false
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bp[5]
	var p []byte
	if hashAlgo == script.HashAlgSha256 {
		p = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, pubkeyHashes.Sha256, script.SigSchemeSecp256k1)
	} else if hashAlgo == script.HashAlgSha512 {
		p = script.PredicatePayToPublicKeyHash(script.HashAlgSha512, pubkeyHashes.Sha512, script.SigSchemeSecp256k1)
	}
	return bytes.Equal(p, bp)
}
