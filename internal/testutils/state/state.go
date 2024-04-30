package teststate

import (
	"testing"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/stretchr/testify/require"
)

func CreateUC(s *state.State, summaryValue uint64, summaryHash []byte) *types.UnicityCertificate{
	roundNumber := uint64(0)
	if s.IsCommitted() {
		roundNumber = s.CommittedUC().GetRoundNumber() + 1
	}

	return &types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  roundNumber,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}
}

func CommitWithUC(t *testing.T, s *state.State) {
	summaryValue, summaryHash, err := s.CalculateRoot()
	if summaryHash == nil {
		summaryHash = make([]byte, s.HashAlgorithm().Size())
	}
	require.NoError(t, err)
	require.NoError(t, s.Commit(CreateUC(s, summaryValue, summaryHash)))
}
