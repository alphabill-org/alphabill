package script

import "fmt"

// PredicateAlwaysTrue is a predicate that evaluates to true with an empty argument.
func PredicateAlwaysTrue() []byte {
	return []byte{StartByte, OpPushBool, BoolTrue}
}

// PredicateAlwaysFalse is a predicate that evaluates to false
func PredicateAlwaysFalse() []byte {
	return []byte{StartByte, OpPushBool, BoolFalse}
}

// PredicatePayToPublicKeyHash is a predicate that evaluates true if a valid signature and public key is given as arguments
func PredicatePayToPublicKeyHash(hashAlg byte, pubKeyHash []byte, sigScheme byte) []byte {
	p := make([]byte, 0, 42)
	p = append(p, StartByte, OpDup, OpHash, hashAlg, OpPushHash, hashAlg)
	p = append(p, pubKeyHash...)
	p = append(p, OpEqual, OpVerify, OpCheckSig, sigScheme)
	return p
}

// PredicatePayToPublicKeyHashDefault same as PredicatePayToPublicKeyHash(sha256, pubkeyHash, secp256k1)
func PredicatePayToPublicKeyHashDefault(pubKeyHash []byte) []byte {
	return PredicatePayToPublicKeyHash(HashAlgSha256, pubKeyHash, SigSchemeSecp256k1)
}

// PredicateArgumentPayToPublicKeyHash creates argument for pay to public key hash predicate.
func PredicateArgumentPayToPublicKeyHash(sig []byte, sigScheme byte, pubKey []byte) []byte {
	s := make([]byte, 0, 103)
	s = append(s, StartByte, OpPushSig, sigScheme)
	s = append(s, sig...)
	s = append(s, OpPushPubKey, sigScheme)
	s = append(s, pubKey...)
	return s
}

// PredicateArgumentPayToPublicKeyHashDefault same as PredicateArgumentPayToPublicKeyHash(sig, secp256k1, pubkey)
func PredicateArgumentPayToPublicKeyHashDefault(sig []byte, pubKey []byte) []byte {
	return PredicateArgumentPayToPublicKeyHash(sig, SigSchemeSecp256k1, pubKey)
}

// PredicateArgumentEmpty predicate argument for PredicateAlwaysTrue
func PredicateArgumentEmpty() []byte {
	return []byte{StartByte}
}

func ExtractPubKeyFromPredicateArgument(predicate []byte) ([]byte, error) {
	if predicate == nil {
		return nil, fmt.Errorf("predicate argument is nil")
	}
	if len(predicate) < 2 || predicate[0] != StartByte {
		return nil, fmt.Errorf("invalid predicate argument")
	}
	for i := 1; i < len(predicate); i++ {
		op, exists := opCodes[predicate[i]]
		if !exists {
			return nil, ErrUnknownOpCode
		}
		dataLength, err := op.getDataLength(predicate[i+1:])
		if err != nil {
			return nil, err
		}
		if op.value == OpPushPubKey {
			return predicate[i+2 : i+1+dataLength], nil
		}
		i += dataLength
	}
	return nil, fmt.Errorf("no public key found in predicate argument")
}
