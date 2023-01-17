package distributed

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/stretchr/testify/require"
)

func TestRoundPipeline_Add(t *testing.T) {
	partitions, err := partition_store.NewPartitionStoreFromGenesis([]*genesis.GenesisPartitionRecord{
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysID1, T2Timeout: 2500}}})
	require.NoError(t, err)
	state := store.RootState{
		LatestRound:    1,
		LatestRootHash: make([]byte, gocrypto.SHA256.Size()),
		Certificates: map[protocol.SystemIdentifier]*certificates.UnicityCertificate{
			protocol.SystemIdentifier(sysID1): {
				InputRecord: inputRecord1,
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
	roundPipe := NewRoundPipeline(gocrypto.SHA256, state, partitions)
	require.NotNil(t, roundPipe)
	require.Equal(t, state.LatestRootHash, roundPipe.GetExecStateId())
	require.NotNil(t, roundPipe.GetHighQc())
	hQC := roundPipe.GetHighQc()
	require.Empty(t, roundPipe.inProgress)
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	changed := map[protocol.SystemIdentifier]*certificates.InputRecord{
		protocol.SystemIdentifier(sysID1): inputRecord2}
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
	require.NotEqual(t, state.LatestRootHash, roundPipe.GetExecStateId())
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
	changed = map[protocol.SystemIdentifier]*certificates.InputRecord{}
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
	require.Equal(t, rootState.LatestRootHash, stateId)
	require.Equal(t, 1, len(rootState.Certificates))
	require.Contains(t, rootState.Certificates, protocol.SystemIdentifier(sysID1))
	// Changes for partition 1 now possible again
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	require.Equal(t, 1, len(roundPipe.statePipeline))
}

func TestRoundPipeline_Reset(t *testing.T) {
	partitions, err := partition_store.NewPartitionStoreFromGenesis([]*genesis.GenesisPartitionRecord{
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysID1, T2Timeout: 2500}}})
	require.NoError(t, err)
	state := store.RootState{
		LatestRound:    1,
		LatestRootHash: make([]byte, gocrypto.SHA256.Size()),
		Certificates: map[protocol.SystemIdentifier]*certificates.UnicityCertificate{
			protocol.SystemIdentifier(sysID1): {
				InputRecord: inputRecord1,
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
	roundPipe := NewRoundPipeline(gocrypto.SHA256, state, partitions)
	require.NotNil(t, roundPipe)
	require.NotNil(t, roundPipe.GetHighQc())
	require.Empty(t, roundPipe.inProgress)
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	changed := map[protocol.SystemIdentifier]*certificates.InputRecord{
		protocol.SystemIdentifier(sysID1): inputRecord2}
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
	require.Empty(t, roundPipe.inProgress)
	require.False(t, roundPipe.IsChangeInPipeline(protocol.SystemIdentifier(sysID1)))
	require.Empty(t, roundPipe.statePipeline)
	// HighQC is will not be reset
	require.Equal(t, qc, roundPipe.GetHighQc())
}
