package distributed

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/stretchr/testify/require"
)

func TestRoundPipeline_Add(t *testing.T) {
	state := &store.RootState{
		Round:    1,
		RootHash: make([]byte, gocrypto.SHA256.Size()),
		Certificates: map[protocol.SystemIdentifier]*certificates.UnicityCertificate{
			protocol.SystemIdentifier(sysID1): {
				InputRecord: inputRecord1,
				UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
					SystemIdentifier:      sysID1,
					SystemDescriptionHash: []byte{1, 2, 3},
				},
				UnicitySeal: &certificates.UnicitySeal{
					RootRoundInfo: &certificates.RootRoundInfo{
						RoundNumber: 1,
					},
					CommitInfo: &certificates.CommitInfo{
						RootHash: make([]byte, gocrypto.SHA256.Size()),
					},
				},
			},
		},
	}
	roundPipe, err := NewRoundPipelineFromRootState(gocrypto.SHA256, state)
	require.NoError(t, err)
	require.NotNil(t, roundPipe)
	require.Equal(t, state.RootHash, roundPipe.GetExecStateId())
	require.NotNil(t, roundPipe.GetHighQc())
	hQC := roundPipe.GetHighQc()
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	changed := map[protocol.SystemIdentifier]*InputData{
		protocol.SystemIdentifier(sysID1): {ir: inputRecord2, sdrh: []byte{1, 2, 3}}}
	newState, err := roundPipe.Add(2, changed)
	require.NoError(t, err)
	require.NotNil(t, newState)
	// try to certify existing round
	newState, err = roundPipe.Add(2, changed)
	require.ErrorContains(t, err, "add state failed: state for round 2 already in pipeline")
	require.Nil(t, newState)
	// Change for partition 1 is in progress, new state is not accepted
	newState, err = roundPipe.Add(3, changed)
	require.ErrorContains(t, err, "add state failed: partition 00000001 has pending changes in pipeline")
	require.Nil(t, newState)
	// Verify exec state id
	require.NotEqual(t, state.RootHash, roundPipe.GetExecStateId())
	// sysid 1 is in progress
	require.True(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	// call update with nil (qc nil, is no-op)
	require.Nil(t, roundPipe.Update(nil))
	require.Equal(t, hQC, roundPipe.GetHighQc())
	stateId := roundPipe.GetExecStateId()
	// simulate QC for round 2
	voteInfo := &certificates.RootRoundInfo{
		RoundNumber:       2,
		Timestamp:         10,
		ParentRoundNumber: 1,
		CurrentRootHash:   roundPipe.GetExecStateId(),
	}
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	// nothing is committed, so expect state returned to be nil
	require.Nil(t, roundPipe.Update(qc))
	// high QC is updated to latest
	require.Equal(t, qc, roundPipe.GetHighQc())
	// round 3 empty block
	changed = map[protocol.SystemIdentifier]*InputData{}
	newState, err = roundPipe.Add(3, changed)
	require.NoError(t, err)
	require.Equal(t, newState, stateId)
	// Round 3 QC and contains commit for round 2
	voteInfo = &certificates.RootRoundInfo{
		RoundNumber:       3,
		Timestamp:         11,
		ParentRoundNumber: 2,
		CurrentRootHash:   roundPipe.GetExecStateId(),
	}
	qc = atomic_broadcast.NewQuorumCertificate(voteInfo, stateId)
	rootState := roundPipe.Update(qc)
	require.NotNil(t, rootState)
	require.Equal(t, rootState.RootHash, stateId)
	require.Equal(t, 1, len(rootState.Certificates))
	require.Contains(t, rootState.Certificates, protocol.SystemIdentifier(sysID1))
	// Changes for partition 1 now possible again
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	require.Equal(t, 1, len(roundPipe.statePipeline))
}

func TestRoundPipeline_Reset(t *testing.T) {
	state := &store.RootState{
		Round:    1,
		RootHash: make([]byte, gocrypto.SHA256.Size()),
		Certificates: map[protocol.SystemIdentifier]*certificates.UnicityCertificate{
			protocol.SystemIdentifier(sysID1): {
				InputRecord: inputRecord1,
				UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
					SystemIdentifier:      sysID1,
					SystemDescriptionHash: []byte{1, 2, 3},
				},
				UnicitySeal: &certificates.UnicitySeal{
					RootRoundInfo: &certificates.RootRoundInfo{
						RoundNumber: 1,
					},
					CommitInfo: &certificates.CommitInfo{
						RootHash: make([]byte, gocrypto.SHA256.Size()),
					},
				},
			},
		},
	}
	roundPipe, err := NewRoundPipelineFromRootState(gocrypto.SHA256, state)
	require.NoError(t, err)
	require.NotNil(t, roundPipe)
	require.NotNil(t, roundPipe.GetHighQc())
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	changed := map[protocol.SystemIdentifier]*InputData{
		protocol.SystemIdentifier(sysID1): {ir: inputRecord2, sdrh: []byte{1, 2, 3}}}
	newState, err := roundPipe.Add(2, changed)
	require.NoError(t, err)
	require.NotNil(t, newState)
	require.True(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	require.Len(t, roundPipe.statePipeline, 1)
	// simulate QC for round 2
	voteInfo := &certificates.RootRoundInfo{
		RoundNumber:       2,
		Timestamp:         10,
		ParentRoundNumber: 1,
		CurrentRootHash:   roundPipe.GetExecStateId(),
	}
	qc := atomic_broadcast.NewQuorumCertificate(voteInfo, nil)
	// nothing is committed, so expect state returned to be nil
	require.Nil(t, roundPipe.Update(qc))
	// high QC is updated to latest
	require.Equal(t, qc, roundPipe.GetHighQc())
	// Reset as if with TC
	roundPipe.Reset(state)
	require.NotNil(t, roundPipe)
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	require.Empty(t, roundPipe.statePipeline)
	// HighQC is will not be reset
	require.Equal(t, qc, roundPipe.GetHighQc())
}
