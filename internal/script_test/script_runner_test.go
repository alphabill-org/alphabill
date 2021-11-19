// seperate package so that we could have access to state package
package script_test

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestAlwaysTrueScript_Ok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, script.OP_PUSH_BOOL, 0x01}
	result := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.True(t, result)
}

func TestP2pkhScript_Ok(t *testing.T) {
	tx := newP2pkhTx(t)
	result := script.RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	assert.True(t, result)
}

func TestEmptyScriptWithTrailingBoolTrue_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, 0x01}
	result := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.False(t, result)
}

func TestOpcodeDataOutOfBounds_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.StartByte, script.OP_PUSH_BOOL} // removed data byte from OP_PUSH_BOOL
	result := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.False(t, result)
}

func TestOpPushHashInvalidType_Nok(t *testing.T) {
	tx := newP2pkhTx(t)
	tx.bearerPredicate[5] = 0x02 // set OP_PUSH_HASH type to sha512

	result := script.RunScript(tx.predicateArgument, tx.bearerPredicate, tx.sigData)
	assert.False(t, result)
}

func TestScriptWithoutFirstByte_Nok(t *testing.T) {
	predicateArgument := []byte{script.StartByte}
	bearerPredicate := []byte{script.OP_PUSH_BOOL, 0x01} // missing start byte
	result := script.RunScript(predicateArgument, bearerPredicate, nil)
	assert.False(t, result)
}

func TestMaxScriptBytes_Nok(t *testing.T) {
	emptyScript := []byte{script.StartByte}
	overfilledScript := createValidScriptWithMinLength(script.MaxScriptBytes + 10)

	r := script.RunScript(emptyScript, overfilledScript, nil)
	assert.False(t, r)

	r = script.RunScript(overfilledScript, emptyScript, nil)
	assert.False(t, r)
}

func createValidScriptWithMinLength(minLength int) []byte {
	s := make([]byte, 0, minLength)
	s = append(s, script.StartByte)

	// fill the script with valid opcodes (OP_PUSH_BOOL TRUE OP_VERIFY)
	for i := 0; i < minLength; i += 3 {
		s = append(s, script.OP_PUSH_BOOL, 0x01, script.OP_VERIFY)
	}
	// add TRUE at the end to make the script valid
	return append(s, script.OP_PUSH_BOOL, 0x01)
}

func newP2pkhTx(t *testing.T) tx {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	verifier, err := signer.Verifier()
	require.NoError(t, err)

	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	pubKeyHash := hash.Sum256(pubKey)

	sigData := getDummyPaymentOrder()
	sig, err := signer.SignBytes(sigData)
	require.NoError(t, err)

	predicateArgument := createPredicateArgument(sig, pubKey)
	bearerPredicate := createBearerPredicate(pubKeyHash)

	return tx{
		sigData:           sigData,
		predicateArgument: predicateArgument,
		bearerPredicate:   bearerPredicate,
	}
}

type tx struct {
	sigData           []byte
	predicateArgument []byte
	bearerPredicate   []byte
}

func getDummyPaymentOrder() []byte {
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

func createBearerPredicate(pubKeyHash []byte) []byte {
	p := make([]byte, 0, 42)
	p = append(p, 0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01)
	p = append(p, pubKeyHash...)
	p = append(p, 0x87, 0x69, 0xac, 0x01)
	return p
}

func createPredicateArgument(sig []byte, pubKey []byte) []byte {
	s := make([]byte, 0, 103)
	s = append(s, 0x53, 0x54, 0x01)
	s = append(s, sig...)
	s = append(s, 0x55, 0x01)
	s = append(s, pubKey...)
	return s
}
