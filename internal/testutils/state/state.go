package teststate

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/state"
	"github.com/stretchr/testify/require"
)

func CreateUC(t *testing.T, s *state.State, summaryValue uint64, summaryHash []byte) *types.UnicityCertificate {
	roundNumber := uint64(0)
	committed, err := s.IsCommitted()
	require.NoError(t, err)
	if committed {
		roundNumber = s.CommittedUC().GetRoundNumber() + 1
	}

	return &types.UnicityCertificate{
		Version: 1,
		InputRecord: &types.InputRecord{
			Version:      1,
			RoundNumber:  roundNumber,
			Hash:         summaryHash,
			SummaryValue: util.Uint64ToBytes(summaryValue),
			Timestamp:    types.NewTimestamp(),
		},
	}
}

func CommitWithUC(t *testing.T, s *state.State) {
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(CreateUC(t, s, summaryValue, summaryHash)))
}
