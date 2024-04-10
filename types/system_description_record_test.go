package types

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/util"
	"github.com/stretchr/testify/require"
)

func TestSystemDescriptionRecord_CanBeHashed(t *testing.T) {
	sdr := &SystemDescriptionRecord{
		SystemIdentifier: 1,
		T2Timeout:        2,
	}
	actualHash := sdr.Hash(gocrypto.SHA256)

	hasher := gocrypto.SHA256.New()
	hasher.Reset()
	hasher.Write(sdr.SystemIdentifier.Bytes())
	hasher.Write(util.Uint64ToBytes(2))
	expectedHash := hasher.Sum(nil)

	require.EqualValues(t, expectedHash, actualHash)
}

func TestSystemDescriptionRecord_IsValid(t *testing.T) {
	t.Run("system description is nil", func(t *testing.T) {
		var s *SystemDescriptionRecord = nil
		require.EqualError(t, s.IsValid(), "system description record is nil")
	})
	t.Run("invalid system identifier", func(t *testing.T) {
		s := &SystemDescriptionRecord{
			SystemIdentifier: 0,
			T2Timeout:        2,
		}
		require.EqualError(t, s.IsValid(), "invalid system identifier: 00000000")
	})
	t.Run("invalid t2 timeout", func(t *testing.T) {
		s := &SystemDescriptionRecord{
			SystemIdentifier: 1,
			T2Timeout:        0,
		}
		require.ErrorIs(t, s.IsValid(), ErrT2TimeoutIsNil)
	})
	t.Run("valid", func(t *testing.T) {
		s := &SystemDescriptionRecord{
			SystemIdentifier: 1,
			T2Timeout:        2500,
		}
		require.NoError(t, s.IsValid())
		require.Equal(t, SystemID(1), s.GetSystemIdentifier())
	})
}
