package _s

//func TestAlwaysTrueScript_Ok(t *testing.T) {
//	predicateArgument := []byte{StartByte}
//	bearerPredicate := PredicateAlwaysTrue()
//	err := RunScript(predicateArgument, bearerPredicate, nil)
//	require.NoError(t, err)
//}
//
//func TestP2pkhScript_Ok(t *testing.T) {
//	tx := newP2pkhTx(t, HashAlgSha256)
//	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
//	require.NoError(t, err)
//}
//
//func TestP2pkhScriptSha512_Ok(t *testing.T) {
//	tx := newP2pkhTx(t, HashAlgSha512)
//	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
//	require.NoError(t, err)
//}
//
//func TestEmptyScriptWithTrailingBoolTrue_Nok(t *testing.T) {
//	predicateArgument := []byte{StartByte}
//	bearerPredicate := []byte{StartByte, BoolTrue}
//	err := RunScript(predicateArgument, bearerPredicate, nil)
//	require.Error(t, err)
//}
//
//func TestOpcodeDataOutOfBounds_Nok(t *testing.T) {
//	predicateArgument := []byte{StartByte}
//	bearerPredicate := []byte{StartByte, OpPushBool} // removed data byte from OP_PUSH_BOOL
//	err := RunScript(predicateArgument, bearerPredicate, nil)
//	require.Error(t, err)
//}
//
//func TestOpPushHashInvalidType_Nok(t *testing.T) {
//	tx := newP2pkhTx(t, HashAlgSha256)
//	tx.bearerPredicate[5] = HashAlgSha512 // set OP_PUSH_HASH type to sha512
//
//	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
//	require.Error(t, err)
//}
//
//func TestScriptWithoutFirstByte_Nok(t *testing.T) {
//	predicateArgument := []byte{StartByte}
//	bearerPredicate := []byte{OpPushBool, BoolTrue} // missing start byte
//	err := RunScript(predicateArgument, bearerPredicate, nil)
//	require.Error(t, err)
//}
//
//func TestMaxScriptBytes_Nok(t *testing.T) {
//	var emptyScript []byte = nil
//	validScript := createValidScriptWithMinLength(MaxScriptBytes - 10)
//	overfilledScript := createValidScriptWithMinLength(MaxScriptBytes + 10)
//
//	// test that script created by createValidScriptWithMinLength can be valid
//	err := RunScript(emptyScript, validScript, nil)
//	require.NoError(t, err)
//	err = RunScript(validScript, emptyScript, nil)
//	require.NoError(t, err)
//
//	err = RunScript(emptyScript, overfilledScript, nil)
//	require.Error(t, err)
//	err = RunScript(overfilledScript, emptyScript, nil)
//	require.Error(t, err)
//}
//
//func createValidScriptWithMinLength(minLength int) []byte {
//	s := make([]byte, 0, minLength)
//	s = append(s, StartByte)
//
//	// fill the script with valid opcodes
//	for i := 0; i < minLength; i += 3 {
//		s = append(s, OpPushBool, BoolTrue, OpVerify)
//	}
//	// add TRUE at the end to make the script valid
//	return append(s, OpPushBool, BoolTrue)
//}
//
//type tx struct {
//	sigData           []byte
//	predicateArgument []byte
//	bearerPredicate   []byte
//}
//
//func newP2pkhTx(t *testing.T, hashAlg byte) tx {
//	txBytes := []byte{1, 2, 3, 4, 5}
//	sig, pubKey := testsig.SignBytes(t, txBytes)
//	pubKeyHash := hashBytes(pubKey, hashAlg)
//
//	predicateArgument := PredicateArgumentPayToPublicKeyHash(sig, SigSchemeSecp256k1, pubKey)
//	bearerPredicate := PredicatePayToPublicKeyHash(hashAlg, pubKeyHash, SigSchemeSecp256k1)
//
//	return tx{
//		sigData:           txBytes,
//		predicateArgument: predicateArgument,
//		bearerPredicate:   bearerPredicate,
//	}
//}
//
//func hashBytes(data []byte, hashAlg byte) []byte {
//	if hashAlg == HashAlgSha256 {
//		return hash.Sum256(data)
//	}
//	if hashAlg == HashAlgSha512 {
//		return hash.Sum512(data)
//	}
//	return data
//}
