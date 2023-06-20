package fc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestFCR_HashIsCalculatedCorrectly(t *testing.T) {
	fcr := &unit.FeeCreditRecord{
		Balance: 1,
		Hash:    test.RandomBytes(32),
		Timeout: 2,
	}
	// calculate actual hash
	hasher := crypto.SHA256.New()
	fcr.Write(hasher)
	actualHash := hasher.Sum(nil)

	// calculate expected hash
	hasher.Reset()
	hasher.Write(util.Uint64ToBytes(uint64(fcr.Balance)))
	hasher.Write(fcr.Hash)
	hasher.Write(util.Uint64ToBytes(fcr.Timeout))
	expectedHash := hasher.Sum(nil)

	require.Equal(t, expectedHash, actualHash)
}

func TestFCR_SummaryValueIsZero(t *testing.T) {
	fcr := &unit.FeeCreditRecord{
		Balance: 1,
		Hash:    test.RandomBytes(32),
		Timeout: 2,
	}
	require.Equal(t, rma.Uint64SummaryValue(0), fcr.Value())
}
