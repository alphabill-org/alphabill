package certification

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
)

func Test_CertificationResponse_IsValid(t *testing.T) {
	validCR := func(t *testing.T) *CertificationResponse {
		cr := &CertificationResponse{
			Partition: 1,
			Shard:     types.ShardID{},
			UC: types.UnicityCertificate{
				Version: 1,
				UnicityTreeCertificate: &types.UnicityTreeCertificate{
					Version:             1,
					PartitionIdentifier: 1,
				},
			},
		}
		require.NoError(t,
			cr.SetTechnicalRecord(TechnicalRecord{
				Round:    99,
				Epoch:    8,
				Leader:   "1",
				StatHash: []byte{1},
				FeeHash:  []byte{2},
			}))

		return cr
	}
	require.NoError(t, validCR(t).IsValid())

	t.Run("nil", func(t *testing.T) {
		var cr *CertificationResponse
		require.EqualError(t, cr.IsValid(), `nil CertificationResponse`)
	})

	t.Run("invalid partition", func(t *testing.T) {
		cr := validCR(t)
		cr.Partition++
		require.EqualError(t, cr.IsValid(), `partition 00000002 doesn't match UnicityTreeCertificate partition 00000001`)

		cr.Partition = 0
		require.EqualError(t, cr.IsValid(), `partition ID is unassigned`)
	})

	t.Run("TechnicalRecord", func(t *testing.T) {
		cr := validCR(t)
		round := cr.Technical.Round
		cr.Technical.Round = 0
		require.EqualError(t, cr.IsValid(), `invalid TechnicalRecord: round is unassigned`)

		// restore round to wrong value - should trigger the hash check
		cr.Technical.Round = round + 1
		require.EqualError(t, cr.IsValid(), `comparing TechnicalRecord hash to UC.TRHash: hash mismatch`)

		cr = validCR(t)
		cr.UC.TRHash = append(cr.UC.TRHash, 0)
		require.EqualError(t, cr.IsValid(), `comparing TechnicalRecord hash to UC.TRHash: hash mismatch`)
	})
}

func Test_CertificationResponse_SetTechnicalRecord(t *testing.T) {
	tr := TechnicalRecord{Round: 123, Epoch: 4, Leader: "567890"}
	cr := CertificationResponse{}
	require.NoError(t, cr.SetTechnicalRecord(tr))
	require.Equal(t, tr, cr.Technical)

	trh, err := tr.Hash()
	require.NoError(t, err)
	require.Equal(t, cr.UC.TRHash, trh)
}
