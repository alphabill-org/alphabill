package unit

import (
	"crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestFCR_HashIsCalculatedCorrectly(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance: 1,
		Hash:    test.RandomBytes(32),
		Timeout: 2,
		Locked:  3,
	}
	// calculate actual hash
	hasher := crypto.SHA256.New()
	fcr.Write(hasher)
	actualHash := hasher.Sum(nil)

	// calculate expected hash
	hasher.Reset()
	hasher.Write(util.Uint64ToBytes(fcr.Balance))
	hasher.Write(fcr.Hash)
	hasher.Write(util.Uint64ToBytes(fcr.Timeout))
	hasher.Write(util.Uint64ToBytes(fcr.Locked))
	expectedHash := hasher.Sum(nil)

	require.Equal(t, expectedHash, actualHash)
}

func TestFCR_SummaryValueIsZero(t *testing.T) {
	fcr := &FeeCreditRecord{
		Balance: 1,
		Hash:    test.RandomBytes(32),
		Timeout: 2,
	}
	require.Equal(t, uint64(0), fcr.SummaryValueInput())
}
