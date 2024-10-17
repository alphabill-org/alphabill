package certification

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func Test_CertificationResponse_IsValid(t *testing.T) {
	validCR := func() *CertificationResponse {
		return &CertificationResponse{
			Partition: 1,
			Shard:     types.ShardID{},
			Technical: TechnicalRecord{
				Round:    99,
				Epoch:    8,
				Leader:   "1",
				StatHash: []byte{1},
				FeeHash:  []byte{2},
			},
		}
	}
	require.NoError(t, validCR().IsValid())

	t.Run("nil", func(t *testing.T) {
		var cr *CertificationResponse
		require.EqualError(t, cr.IsValid(), `nil CertificationResponse`)
	})

	t.Run("invalid partition", func(t *testing.T) {
		cr := validCR()
		cr.Partition = 0
		require.EqualError(t, cr.IsValid(), `partition ID is unassigned`)
	})

	t.Run("TechnicalRecord", func(t *testing.T) {
		cr := validCR()
		cr.Technical.Round = 0
		require.EqualError(t, cr.IsValid(), `invalid TechnicalRecord: round is unassigned`)
	})
}
