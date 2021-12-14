// seperate package so that we could have access to state package
package script

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

func TestAlwaysTrueScript_Ok(t *testing.T) {
	predicateArgument := []byte{StartByte}
	bearerPredicate := PredicateAlwaysTrue()
	err := RunScript(predicateArgument, bearerPredicate, nil)
	require.Nil(t, err)
}

func TestP2pkhScript_Ok(t *testing.T) {
	tx := newP2pkhTx(t, HashAlgSha256)
	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	require.Nil(t, err)
}

func TestP2pkhScriptSha512_Ok(t *testing.T) {
	tx := newP2pkhTx(t, HashAlgSha512)
	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	require.Nil(t, err)
}

func TestEmptyScriptWithTrailingBoolTrue_Nok(t *testing.T) {
	predicateArgument := []byte{StartByte}
	bearerPredicate := []byte{StartByte, BoolTrue}
	err := RunScript(predicateArgument, bearerPredicate, nil)
	require.NotNil(t, err)
}

func TestOpcodeDataOutOfBounds_Nok(t *testing.T) {
	predicateArgument := []byte{StartByte}
	bearerPredicate := []byte{StartByte, OpPushBool} // removed data byte from OP_PUSH_BOOL
	err := RunScript(predicateArgument, bearerPredicate, nil)
	require.NotNil(t, err)
}

func TestOpPushHashInvalidType_Nok(t *testing.T) {
	tx := newP2pkhTx(t, HashAlgSha256)
	tx.bearerPredicate[5] = HashAlgSha512 // set OP_PUSH_HASH type to sha512

	err := RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	require.NotNil(t, err)
}

func TestScriptWithoutFirstByte_Nok(t *testing.T) {
	predicateArgument := []byte{StartByte}
	bearerPredicate := []byte{OpPushBool, BoolTrue} // missing start byte
	err := RunScript(predicateArgument, bearerPredicate, nil)
	require.NotNil(t, err)
}

func TestMaxScriptBytes_Nok(t *testing.T) {
	emptyScript := []byte{StartByte}
	validScript := createValidScriptWithMinLength(MaxScriptBytes - 10)
	overfilledScript := createValidScriptWithMinLength(MaxScriptBytes + 10)

	// test that script creaated by createValidScriptWithMinLength can be valid
	err := RunScript(emptyScript, validScript, nil)
	require.Nil(t, err)
	err = RunScript(validScript, emptyScript, nil)
	require.Nil(t, err)

	err = RunScript(emptyScript, overfilledScript, nil)
	require.NotNil(t, err)
	err = RunScript(overfilledScript, emptyScript, nil)
	require.NotNil(t, err)
}

func createValidScriptWithMinLength(minLength int) []byte {
	s := make([]byte, 0, minLength)
	s = append(s, StartByte)

	// fill the script with valid opcodes
	for i := 0; i < minLength; i += 3 {
		s = append(s, OpPushBool, BoolTrue, OpVerify)
	}
	// add TRUE at the end to make the script valid
	return append(s, OpPushBool, BoolTrue)
}

type tx struct {
	sigData           []byte
	predicateArgument []byte
	bearerPredicate   []byte
}

func newP2pkhTx(t *testing.T, hashAlg byte) tx {
	po := createDummyPaymentOrder()
	sig, pubKey := testsig.SignBytes(t, po)
	pubKeyHash := hashBytes(pubKey, hashAlg)

	predicateArgument := PredicateArgumentPayToPublicKeyHash(sig, SigSchemeSecp256k1, pubKey)
	bearerPredicate := PredicatePayToPublicKeyHash(hashAlg, pubKeyHash, SigSchemeSecp256k1)

	return tx{
		sigData:           po,
		predicateArgument: predicateArgument,
		bearerPredicate:   bearerPredicate,
	}
}

func hashBytes(data []byte, hashAlg byte) []byte {
	if hashAlg == HashAlgSha256 {
		return hash.Sum256(data)
	}
	if hashAlg == HashAlgSha512 {
		return hash.Sum512(data)
	}
	return data
}

func createDummyPaymentOrder() []byte {
	po := domain.PaymentOrder{
		BillID:            1,
		Type:              domain.PaymentTypeTransfer,
		Amount:            0,
		Backlink:          []byte{},
		PayeePredicate:    []byte{},
		PredicateArgument: []byte{},
	}
	return po.Bytes()
}
