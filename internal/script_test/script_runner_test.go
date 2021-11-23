// seperate package so that we could have access to state package
package script_test

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAlwaysTrueScript_Ok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, script.OpPushBool, script.BoolTrue}
	err := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.Nil(t, err)
}

func TestP2pkhScript_Ok(t *testing.T) {
	tx := newP2pkhTx(t, script.HashAlgSha256)
	err := script.RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	assert.Nil(t, err)
}

func TestP2pkhScriptSha512_Ok(t *testing.T) {
	tx := newP2pkhTx(t, script.HashAlgSha512)
	err := script.RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	assert.Nil(t, err)
}

func TestEmptyScriptWithTrailingBoolTrue_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, script.BoolTrue}
	err := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.NotNil(t, err)
}

func TestOpcodeDataOutOfBounds_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, script.OpPushBool} // removed data byte from OP_PUSH_BOOL
	err := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.NotNil(t, err)
}

func TestOpPushHashInvalidType_Nok(t *testing.T) {
	tx := newP2pkhTx(t, script.HashAlgSha256)
	tx.bearerPredicate[5] = script.HashAlgSha512 // set OP_PUSH_HASH type to sha512

	err := script.RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	assert.NotNil(t, err)
}

func TestScriptWithoutFirstByte_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.OpPushBool, script.BoolTrue} // missing start byte
	err := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.NotNil(t, err)
}

func TestMaxScriptBytes_Nok(t *testing.T) {
	emptyScript := []byte{script.StartByte}
	validScript := createValidScriptWithMinLength(script.MaxScriptBytes - 10)
	overfilledScript := createValidScriptWithMinLength(script.MaxScriptBytes + 10)

	// test that script creaated by createValidScriptWithMinLength can be valid
	err := script.RunScript(emptyScript, validScript, nil)
	assert.Nil(t, err)
	err = script.RunScript(validScript, emptyScript, nil)
	assert.Nil(t, err)

	err = script.RunScript(emptyScript, overfilledScript, nil)
	assert.NotNil(t, err)
	err = script.RunScript(overfilledScript, emptyScript, nil)
	assert.NotNil(t, err)
}

func createValidScriptWithMinLength(minLength int) []byte {
	s := make([]byte, 0, minLength)
	s = append(s, script.StartByte)

	// fill the script with valid opcodes
	for i := 0; i < minLength; i += 3 {
		s = append(s, script.OpPushBool, script.BoolTrue, script.OpVerify)
	}
	// add TRUE at the end to make the script valid
	return append(s, script.OpPushBool, script.BoolTrue)
}

type tx struct {
	sigData           []byte
	predicateArgument []byte
	bearerPredicate   []byte
}

func newP2pkhTx(t *testing.T, hashAlg byte) tx {
	po := createDummyPaymentOrder()
	sig, pubKey := testsig.SignBytes(t, po)
	pubKeyHash := hashData(pubKey, hashAlg)

	predicateArgument := createPredicateArgument(sig, pubKey)
	bearerPredicate := createBearerPredicate(hashAlg, pubKeyHash)

	return tx{
		sigData:           po,
		predicateArgument: predicateArgument,
		bearerPredicate:   bearerPredicate,
	}
}

func hashData(data []byte, hashAlg byte) []byte {
	if hashAlg == script.HashAlgSha256 {
		return hash.Sum256(data)
	}
	if hashAlg == script.HashAlgSha512 {
		return hash.Sum512(data)
	}
	return data
}

func createBearerPredicate(hashType byte, pubKeyHash []byte) []byte {
	p := make([]byte, 0, 42)
	p = append(p, script.StartByte, script.OpDup, script.OpHash, hashType, script.OpPushHash, hashType)
	p = append(p, pubKeyHash...)
	p = append(p, script.OpEqual, script.OpVerify, script.OpCheckSig, script.SigSchemeSecp256k1)
	return p
}

func createPredicateArgument(sig []byte, pubKey []byte) []byte {
	s := make([]byte, 0, 103)
	s = append(s, script.StartByte, script.OpPushSig, script.SigSchemeSecp256k1)
	s = append(s, sig...)
	s = append(s, script.OpPushPubKey, script.HashAlgSha256)
	s = append(s, pubKey...)
	return s
}

func createDummyPaymentOrder() []byte {
	po := state.PaymentOrder{
		BillID:            1,
		Type:              state.PaymentTypeTransfer,
		JoinBillId:        1,
		Amount:            0,
		Backlink:          []byte{},
		PayeePredicate:    []byte{},
		PredicateArgument: []byte{},
	}
	return po.SigBytes()
}
