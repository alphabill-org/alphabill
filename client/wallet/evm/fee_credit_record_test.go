package evm

import (
	"encoding/hex"
	"testing"

	test "github.com/alphabill-org/alphabill/validator/pkg/testutils"
	"github.com/stretchr/testify/require"
)

var pubKey = [33]byte{0x03,
	0x4F, 0x93, 0xA5, 0x2E, 0xE2, 0x16, 0x4C, 0xFC, 0x48, 0xC2, 0x87, 0x62, 0x57, 0xCC, 0xF0, 0x43,
	0xAD, 0x71, 0xCF, 0x5A, 0x5D, 0x6B, 0x4B, 0x1F, 0x2F, 0x19, 0xA, 0xDC, 0xBF, 0x90, 0x1C, 0x56}

func TestFeeCreditRecordIDFromPublicKey(t *testing.T) {
	unitID, err := hex.DecodeString("276B52B4808893d1e2Affd5310898818E8e7699d")
	require.NoError(t, err)
	// shard part can be nil
	require.EqualValues(t, unitID, FeeCreditRecordIDFromPublicKey(nil, pubKey[:]))
	// or any other value, evm will ignore this
	require.EqualValues(t, unitID, FeeCreditRecordIDFromPublicKey(test.RandomBytes(2), pubKey[:]))
	// if pubkey is nil, unitID returns O address
	require.EqualValues(t, make([]byte, 20), FeeCreditRecordIDFromPublicKey(nil, nil))
}

func Test_generateAddress(t *testing.T) {
	// nil, returns empty address
	addr, err := generateAddress(nil)
	require.Error(t, err)
	require.EqualValues(t, make([]byte, 20), addr)
	// not an actual public key
	invalidPubKey, err := hex.DecodeString("276B52B4808893d1e2Affd5310898818E8e7699d")
	require.NoError(t, err)
	addr, err = generateAddress(invalidPubKey)
	require.Error(t, err)
	require.EqualValues(t, make([]byte, 20), addr)
	// ok
	addr, err = generateAddress(pubKey[:])
	require.NoError(t, err)
	expected, err := hex.DecodeString("276B52B4808893d1e2Affd5310898818E8e7699d")
	require.NoError(t, err)
	require.EqualValues(t, expected, addr)
}
