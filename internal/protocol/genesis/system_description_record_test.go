package genesis

import (
	gocrypto "crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func TestSystemDescriptionRecord_CanBeHashed(t *testing.T) {
	sdr := &SystemDescriptionRecord{
		SystemIdentifier: []byte{1},
		T2Timeout:        2,
	}

	hasher := gocrypto.SHA256.New()
	actualHash := sdr.Hash(gocrypto.SHA256)

	hasher.Reset()
	hasher.Write([]byte{1})
	hasher.Write(util.Uint64ToBytes(2))
	expectedHash := hasher.Sum(nil)

	require.EqualValues(t, expectedHash, actualHash)
}
