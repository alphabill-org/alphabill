package script

// PredicateAlwaysTrue is a predicate that evaluates to true with an empty argument.
func PredicateAlwaysTrue() []byte {
	return []byte{StartByte, OpPushBool, BoolTrue}
}
