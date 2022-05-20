package money

import (
	"bytes"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
)

// verifyOwner checks if given p2pkh bearer predicate contains given pubKey hash
func verifyOwner(pubKeyHash *wallet.ShaHashes, bp []byte) (bool, error) {
	// p2pkh predicate: [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
	// p2pkh predicate: [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]

	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bp) != 42 && len(bp) != 74 {
		return false, nil
	}
	// 5th byte is PushHash 0x4f
	if bp[4] != 0x4f {
		return false, nil
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bp[5]
	if hashAlgo == 0x01 {
		return bytes.Equal(bp[6:38], pubKeyHash.Sha256), nil
	} else if hashAlgo == 0x02 {
		return bytes.Equal(bp[6:70], pubKeyHash.Sha512), nil
	}
	return false, nil
}
