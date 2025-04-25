package consensus

import (
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

type (
	MockState struct {
		inProgress   []types.PartitionID
		irInProgress *types.InputRecord
		shardInfo    func(partition types.PartitionID, shard types.ShardID) *storage.ShardInfo
	}
)

var irSysID1 = &types.InputRecord{
	Version:         1,
	PreviousHash:    []byte{1, 1, 1},
	Hash:            []byte{2, 2, 2},
	BlockHash:       []byte{3, 3, 3},
	SummaryValue:    []byte{4, 4, 4},
	RoundNumber:     1,
	SumOfEarnedFees: 0,
}

func (s *MockState) GetCertificates() []*types.UnicityCertificate {
	return []*types.UnicityCertificate{{
		Version:                1,
		InputRecord:            irSysID1,
		UnicityTreeCertificate: &types.UnicityTreeCertificate{Partition: 1},
		UnicitySeal: &types.UnicitySeal{
			Version:              1,
			RootChainRoundNumber: 1,
		},
	},
	}
}

func (s *MockState) ShardInfo(partition types.PartitionID, shard types.ShardID) *storage.ShardInfo {
	return s.shardInfo(partition, shard)
}

func (s *MockState) IsChangeInProgress(id types.PartitionID, _ types.ShardID) *types.InputRecord {
	for _, sysId := range s.inProgress {
		if sysId == id {
			return s.irInProgress
		}
	}
	return nil
}

func TestIRChangeReqVerifier_VerifyIRChangeReq(t *testing.T) {
	tn := testutils.NewTestNode(t)
	signer := tn.Signer

	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     partitionID1,
		ShardID:         types.ShardID{},
		PartitionTypeID: 1,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2000 * time.Millisecond,
		Validators:      []*types.NodeInfo{tn.NodeInfo(t)},
		Epoch:           0,
		EpochStart:      1,
	}
	orchestration := testpartition.NewOrchestration(t, logger.New(t))
	require.NoError(t, orchestration.AddShardConfig(shardConf))

	stateProvider := func(partitionIDs []types.PartitionID, irs *types.InputRecord) *MockState {
		return &MockState{
			inProgress:   partitionIDs,
			irInProgress: irs,
			shardInfo: func(partitionID types.PartitionID, shard types.ShardID) *storage.ShardInfo {
				if partitionID != shardConf.PartitionID {
					return nil
				}
				si, err := storage.NewShardInfo(shardConf, crypto.SHA256)
				si.LastCR = &certification.CertificationResponse{
					UC: types.UnicityCertificate{
						UnicitySeal: &types.UnicitySeal{
							RootChainRoundNumber: 2,
							Timestamp:            1,
						},
						InputRecord: &types.InputRecord{},
					},
					Technical: certification.TechnicalRecord{
						Round: 2,
					},
				}
				si.RootHash = irSysID1.Hash
				require.NoError(t, err)
				return si
			},
		}
	}

	t.Run("ir change request nil", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond},
			state:  stateProvider([]types.PartitionID{partitionID1}, nil),
		}
		data, err := ver.VerifyIRChangeReq(2, nil)
		require.Nil(t, data)
		require.EqualError(t, err, "IR change request is nil")
	})

	t.Run("error change in progress", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond},
			state:  stateProvider([]types.PartitionID{partitionID1}, &types.InputRecord{Version: 1}),
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       1,
		}
		request := &certification.BlockCertificationRequest{
			PartitionID: partitionID1,
			NodeID:      tn.PeerConf.ID.String(),
			InputRecord: newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  partitionID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "shard 00000001- has pending changes in pipeline")
	})

	t.Run("invalid partition ID", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond},
			state:  stateProvider([]types.PartitionID{partitionID1}, &types.InputRecord{Version: 1}),
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       1,
		}
		request := &certification.BlockCertificationRequest{
			PartitionID: partitionID2,
			NodeID:      tn.PeerConf.ID.String(),
			InputRecord: newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  partitionID2,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "missing shard info for partition 2 shard ")
	})

	t.Run("duplicate request", func(t *testing.T) {
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       1,
		}
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond},
			state:  stateProvider([]types.PartitionID{partitionID1}, newIR),
		}
		request := &certification.BlockCertificationRequest{
			PartitionID: partitionID1,
			NodeID:      tn.PeerConf.ID.String(),
			InputRecord: newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  partitionID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(1, irChReq)
		require.Nil(t, data)
		require.ErrorIs(t, err, ErrDuplicateChangeReq)
	})

	t.Run("invalid root round, luc round is bigger", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:  stateProvider(nil, nil),
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       1,
		}
		request := &certification.BlockCertificationRequest{
			PartitionID: partitionID1,
			NodeID:      tn.PeerConf.ID.String(),
			InputRecord: newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  partitionID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(1, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "current round 1 is in the past, LUC round 2")
	})

	t.Run("ok", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params: &Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:  stateProvider(nil, nil),
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       1,
		}
		request := &certification.BlockCertificationRequest{
			PartitionID: partitionID1,
			NodeID:      tn.PeerConf.ID.String(),
			InputRecord: newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  partitionID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		ir, err := ver.VerifyIRChangeReq(2, irChReq)
		require.NoError(t, err)
		require.Equal(t, newIR, ir)
	})
}

func TestNewIRChangeReqVerifier(t *testing.T) {
	t.Run("state monitor is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&Parameters{}, nil)
		require.EqualError(t, err, "state monitor is nil")
		require.Nil(t, ver)
	})

	t.Run("params is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(nil, &MockState{})
		require.EqualError(t, err, "consensus params is nil")
		require.Nil(t, ver)
	})

	t.Run("ok", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&Parameters{}, &MockState{})
		require.NoError(t, err)
		require.NotNil(t, ver)
	})
}

func TestNewLucBasedT2TimeoutGenerator(t *testing.T) {
	t.Run("state monitor is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&Parameters{}, nil)
		require.EqualError(t, err, "state monitor is nil")
		require.Nil(t, tmoGen)
	})

	t.Run("params is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(nil, &MockState{})
		require.EqualError(t, err, "consensus params is nil")
		require.Nil(t, tmoGen)
	})

	t.Run("ok", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&Parameters{}, &MockState{})
		require.NoError(t, err)
		require.NotNil(t, tmoGen)
	})
}

func TestPartitionTimeoutGenerator_GetT2Timeouts(t *testing.T) {
	tn := testutils.NewTestNode(t)
	shardConf := &types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: partitionID1,
		T2Timeout:   2500 * time.Millisecond,
		Validators:  []*types.NodeInfo{tn.NodeInfo(t)},
	}
	state := &MockState{
		shardInfo: func(partition types.PartitionID, shard types.ShardID) *storage.ShardInfo {
			si, err := storage.NewShardInfo(shardConf, crypto.SHA256)
			require.NoError(t, err)
			return si
		},
	}
	tmoGen := &PartitionTimeoutGenerator{
		blockRate: 500 * time.Millisecond,
		state:     state,
	}

	// last certified round is 1 then 11 - 1 = 10 we have not heard from partition in 10 rounds ~ at minimum 2500 ms not yet timeout
	tmos, err := tmoGen.GetT2Timeouts(11)
	require.NoError(t, err)
	require.Empty(t, tmos)
	// last certified round is 1 then 12 - 1 = 11 we have not heard from partition in 12 rounds ~ at minimum 2750 ms not yet timeout
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.Len(t, tmos, 1)
	require.EqualValues(t, tmos[0].GetPartitionID(), partitionID1)
	// mock sysID1 has pending change in pipeline - no timeout will be generated
	state.inProgress = append(state.inProgress, partitionID1)
	state.irInProgress = irSysID1
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.Empty(t, tmos)
}
