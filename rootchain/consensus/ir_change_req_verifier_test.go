package consensus

import (
	"crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	testcertificates "github.com/alphabill-org/alphabill/internal/testutils/certificates"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	testpartition "github.com/alphabill-org/alphabill/rootchain/partitions/testutils"
	"github.com/stretchr/testify/require"
)

type (
	MockState struct {
		inProgress   []types.PartitionID
		irInProgress *types.InputRecord
		shardInfo    func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error)
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

func (s *MockState) ShardInfo(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
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
	signer, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	pdr := &types.PartitionDescriptionRecord{
		Version:             1,
		NetworkIdentifier:   5,
		PartitionIdentifier: 1,
		T2Timeout:           2000 * time.Millisecond,
	}
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			PartitionDescription: pdr,
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: *pdr},
			},
			Certificate: testcertificates.CreateUnicityCertificate(t, signer, irSysID1, pdr, 1, make([]byte, 32), make([]byte, 32)),
		},
	}
	orchestration := testpartition.NewOrchestration(t, &genesis.RootGenesis{Version: 1, Partitions: genesisPartitions})
	stateProvider := func(partitionIDs []types.PartitionID, irs *types.InputRecord) *MockState {
		return &MockState{
			inProgress:   partitionIDs,
			irInProgress: irs,
			shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
				return storage.NewShardInfoFromGenesis(genesisPartitions[0])
			},
		}
	}

	t.Run("ir change request nil", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond},
			state:         stateProvider([]types.PartitionID{sysID1}, nil),
			orchestration: orchestration,
		}
		data, err := ver.VerifyIRChangeReq(2, nil)
		require.Nil(t, data)
		require.EqualError(t, err, "IR change request is nil")
	})

	t.Run("error change in progress", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond},
			state:         stateProvider([]types.PartitionID{sysID1}, &types.InputRecord{Version: 1}),
			orchestration: orchestration,
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       genesisPartitions[0].Certificate.UnicitySeal.Timestamp,
		}
		request := &certification.BlockCertificationRequest{
			Partition:      sysID1,
			NodeIdentifier: "node1",
			InputRecord:    newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  sysID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "add state failed: partition 00000001 has pending changes in pipeline")
	})

	t.Run("invalid partition ID", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond},
			state:         stateProvider([]types.PartitionID{sysID1}, &types.InputRecord{Version: 1}),
			orchestration: orchestration,
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       genesisPartitions[0].Certificate.UnicitySeal.Timestamp,
		}
		request := &certification.BlockCertificationRequest{
			Partition:      sysID2,
			NodeIdentifier: "node1",
			InputRecord:    newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  sysID2,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "acquiring partition genesis: partition genesis not found for partition 2")
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
			Timestamp:       genesisPartitions[0].Certificate.UnicitySeal.Timestamp,
		}
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond},
			state:         stateProvider([]types.PartitionID{sysID1}, newIR),
			orchestration: orchestration,
		}
		request := &certification.BlockCertificationRequest{
			Partition:      sysID1,
			NodeIdentifier: "node1",
			InputRecord:    newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  sysID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(1, irChReq)
		require.Nil(t, data)
		require.ErrorIs(t, err, ErrDuplicateChangeReq)
	})

	t.Run("invalid root round, luc round is bigger", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:         stateProvider(nil, nil),
			orchestration: orchestration,
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       genesisPartitions[0].Certificate.UnicitySeal.Timestamp,
		}
		request := &certification.BlockCertificationRequest{
			Partition:      sysID1,
			NodeIdentifier: "node1",
			InputRecord:    newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  sysID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(0, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "current round 0 is in the past, LUC round 1")
	})

	t.Run("ok", func(t *testing.T) {
		ver := &IRChangeReqVerifier{
			params:        &Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:         stateProvider(nil, nil),
			orchestration: orchestration,
		}
		newIR := &types.InputRecord{
			Version:         1,
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
			Timestamp:       genesisPartitions[0].Certificate.UnicitySeal.Timestamp,
		}
		request := &certification.BlockCertificationRequest{
			Partition:      sysID1,
			NodeIdentifier: "node1",
			InputRecord:    newIR,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			Partition:  sysID1,
			CertReason: abtypes.Quorum,
			Requests:   []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.NoError(t, err)
		require.Equal(t, newIR, data.IR)
		require.Equal(t, sysID1, data.Partition)
	})
}

func TestNewIRChangeReqVerifier(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			PartitionDescription: &types.PartitionDescriptionRecord{
				Version:             1,
				NetworkIdentifier:   5,
				PartitionIdentifier: 1,
				T2Timeout:           2600 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1}},
				{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1}},
				{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1}},
			},
			Certificate: &types.UnicityCertificate{
				InputRecord: &types.InputRecord{},
			},
		},
	}
	orchestration := testpartition.NewOrchestration(t, &genesis.RootGenesis{Version: 1, Partitions: genesisPartitions})

	t.Run("orchestration is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&Parameters{}, nil, &MockState{})
		require.EqualError(t, err, "orchestration is nil")
		require.Nil(t, ver)
	})

	t.Run("state monitor is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&Parameters{}, orchestration, nil)
		require.EqualError(t, err, "state monitor is nil")
		require.Nil(t, ver)
	})

	t.Run("params is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(nil, orchestration, &MockState{})
		require.EqualError(t, err, "consensus params is nil")
		require.Nil(t, ver)
	})

	t.Run("ok", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&Parameters{}, orchestration, &MockState{})
		require.NoError(t, err)
		require.NotNil(t, ver)
	})
}

func TestNewLucBasedT2TimeoutGenerator(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			PartitionDescription: &types.PartitionDescriptionRecord{
				Version:             1,
				NetworkIdentifier:   5,
				PartitionIdentifier: 1,
				T2Timeout:           2600 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1}},
			},
			Certificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{}},
		},
	}

	orchestration := testpartition.NewOrchestration(t, &genesis.RootGenesis{Version: 1, Partitions: genesisPartitions})

	t.Run("state monitor is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&Parameters{}, orchestration, nil)
		require.EqualError(t, err, "state monitor is nil")
		require.Nil(t, tmoGen)
	})

	t.Run("orchestration is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&Parameters{}, nil, &MockState{})
		require.EqualError(t, err, "orchestration is nil")
		require.Nil(t, tmoGen)
	})

	t.Run("params is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(nil, orchestration, &MockState{})
		require.EqualError(t, err, "consensus params is nil")
		require.Nil(t, tmoGen)
	})

	t.Run("ok", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&Parameters{}, orchestration, &MockState{})
		require.NoError(t, err)
		require.NotNil(t, tmoGen)
	})
}

func TestPartitionTimeoutGenerator_GetT2Timeouts(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			Version: 1,
			Certificate: &types.UnicityCertificate{
				Version:                1,
				InputRecord:            &types.InputRecord{Version: 1},
				UnicitySeal:            &types.UnicitySeal{Version: 1, RootChainRoundNumber: 1},
				UnicityTreeCertificate: &types.UnicityTreeCertificate{Partition: sysID1},
			},
			PartitionDescription: &types.PartitionDescriptionRecord{
				Version:             1,
				NetworkIdentifier:   5,
				PartitionIdentifier: sysID1,
				T2Timeout:           2500 * time.Millisecond,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes, PartitionDescriptionRecord: types.PartitionDescriptionRecord{Version: 1}},
			},
		},
	}
	orchestration := testpartition.NewOrchestration(t, &genesis.RootGenesis{Version: 1, Partitions: genesisPartitions})
	state := &MockState{
		shardInfo: func(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error) {
			return storage.NewShardInfoFromGenesis(genesisPartitions[0])
		},
	}
	tmoGen := &PartitionTimeoutGenerator{
		blockRate:     500 * time.Millisecond,
		state:         state,
		orchestration: orchestration,
	}
	var tmos []types.PartitionID
	// last certified round is 1 then 11 - 1 = 10 we have not heard from partition in 10 rounds ~ at minimum 2500 ms not yet timeout
	tmos, err = tmoGen.GetT2Timeouts(11)
	require.NoError(t, err)
	require.Empty(t, tmos)
	// last certified round is 1 then 12 - 1 = 11 we have not heard from partition in 12 rounds ~ at minimum 2750 ms not yet timeout
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.EqualValues(t, []types.PartitionID{sysID1}, tmos)
	// mock sysID1 has pending change in pipeline - no timeout will be generated
	state.inProgress = append(state.inProgress, sysID1)
	state.irInProgress = irSysID1
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.Empty(t, tmos)
}
