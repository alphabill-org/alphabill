package script

// PredicateAlwaysTrue is a predicate that evaluates to true with an empty argument.
func PredicateAlwaysTrue() []byte {
	return []byte{StartByte, OpPushBool, BoolTrue}
}

// PredicatePayToPublicKeyHash is a predicate that evaluates true if a valid signature and public key is given as arguments
func PredicatePayToPublicKeyHash(hashAlg byte, pubKeyHash []byte, sigScheme byte) []byte {
	p := make([]byte, 0, 42)
	p = append(p, StartByte, OpDup, OpHash, hashAlg, OpPushHash, hashAlg)
	p = append(p, pubKeyHash...)
	p = append(p, OpEqual, OpVerify, OpCheckSig, sigScheme)
	return p
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
