package unit

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/testutils"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestFCR_HashIsCalculatedCorrectly(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: test.RandomBytes(32),
		Timeout:  2,
		Locked:   3,
	}
	// calculate actual hash
	hasher := crypto.SHA256.New()
	require.NoError(t, fcr.Write(hasher))
	actualHash := hasher.Sum(nil)

	// calculate expected hash
	hasher.Reset()
	res, err := cbor.Marshal(fcr)
	require.NoError(t, err)
	hasher.Write(res)
	expectedHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
	// check all fields serialized
	var fcrFromSerialized FeeCreditRecord
	require.NoError(t, cbor.Unmarshal(res, &fcrFromSerialized))
	require.Equal(t, fcr, &fcrFromSerialized)
}

func TestFCR_SummaryValueIsZero(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance:  1,
		Backlink: test.RandomBytes(32),
		Timeout:  2,
	}
	require.Equal(t, uint64(0), fcr.SummaryValueInput())
}
