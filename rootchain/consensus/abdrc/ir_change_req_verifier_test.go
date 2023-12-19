package abdrc

import (
	"crypto"
	"fmt"
	"testing"
	"time"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

type (
	MockState struct {
		inProgress   []types.SystemID32
		irInProgress *types.InputRecord
	}
)

var irSysID1 = &types.InputRecord{
	PreviousHash:    []byte{1, 1, 1},
	Hash:            []byte{2, 2, 2},
	BlockHash:       []byte{3, 3, 3},
	SummaryValue:    []byte{4, 4, 4},
	RoundNumber:     1,
	SumOfEarnedFees: 0,
}

func (s *MockState) GetCertificates() map[types.SystemID32]*types.UnicityCertificate {
	return map[types.SystemID32]*types.UnicityCertificate{
		types.SystemID32(1): {
			InputRecord:            irSysID1,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{},
			UnicitySeal: &types.UnicitySeal{
				RootChainRoundNumber: 1,
			},
		},
	}
}

func (s *MockState) GetCertificate(id types.SystemID32) (*types.UnicityCertificate, error) {
	cm := s.GetCertificates()
	if uc, ok := cm[id]; ok {
		return uc, nil
	}
	return nil, fmt.Errorf("no UC for partition %s", id)
}

func (s *MockState) IsChangeInProgress(id types.SystemID32) *types.InputRecord {
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
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: []byte{0, 0, 0, 1},
				T2Timeout:        2000,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	t.Run("ir change request nil", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond},
			state:      &MockState{inProgress: []types.SystemID32{sysID1}, irInProgress: &types.InputRecord{}},
			partitions: pConf,
		}
		data, err := ver.VerifyIRChangeReq(2, nil)
		require.Nil(t, data)
		require.EqualError(t, err, "IR change request is nil")
	})
	t.Run("error change in progress", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond},
			state:      &MockState{inProgress: []types.SystemID32{sysID1}, irInProgress: &types.InputRecord{}},
			partitions: pConf,
		}
		newIR := &types.InputRecord{
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
		}
		request := &certification.BlockCertificationRequest{
			SystemIdentifier: sysID1.ToSystemID(),
			NodeIdentifier:   "node1",
			InputRecord:      newIR,
			RootRoundNumber:  1,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			SystemIdentifier: sysID1,
			CertReason:       abtypes.Quorum,
			Requests:         []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "add state failed: partition 00000001 has pending changes in pipeline")
	})
	t.Run("invalid sys ID", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond},
			state:      &MockState{inProgress: []types.SystemID32{sysID1}, irInProgress: &types.InputRecord{}},
			partitions: pConf,
		}
		newIR := &types.InputRecord{
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
		}
		request := &certification.BlockCertificationRequest{
			SystemIdentifier: sysID2.ToSystemID(),
			NodeIdentifier:   "node1",
			InputRecord:      newIR,
			RootRoundNumber:  1,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			SystemIdentifier: sysID2,
			CertReason:       abtypes.Quorum,
			Requests:         []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "reading partition certificate: no UC for partition 00000002")
	})
	t.Run("duplicate request", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		newIR := &types.InputRecord{
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
		}
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond},
			state:      &MockState{inProgress: []types.SystemID32{sysID1}, irInProgress: newIR},
			partitions: pConf,
		}
		request := &certification.BlockCertificationRequest{
			SystemIdentifier: sysID1.ToSystemID(),
			NodeIdentifier:   "node1",
			InputRecord:      newIR,
			RootRoundNumber:  1,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			SystemIdentifier: sysID1,
			CertReason:       abtypes.Quorum,
			Requests:         []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(1, irChReq)
		require.Nil(t, data)
		require.ErrorIs(t, err, ErrDuplicateChangeReq)
	})
	t.Run("invalid root round, luc round is bigger", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:      &MockState{},
			partitions: pConf,
		}
		newIR := &types.InputRecord{
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
		}
		request := &certification.BlockCertificationRequest{
			SystemIdentifier: sysID1.ToSystemID(),
			NodeIdentifier:   "node1",
			InputRecord:      newIR,
			RootRoundNumber:  1,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			SystemIdentifier: sysID1,
			CertReason:       abtypes.Quorum,
			Requests:         []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(0, irChReq)
		require.Nil(t, data)
		require.EqualError(t, err, "current round 0 is in the past, LUC round 1")
	})
	t.Run("ok", func(t *testing.T) {
		pConf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver := &IRChangeReqVerifier{
			params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond, HashAlgorithm: crypto.SHA256},
			state:      &MockState{},
			partitions: pConf,
		}
		newIR := &types.InputRecord{
			PreviousHash:    irSysID1.Hash,
			Hash:            []byte{3, 3, 3},
			BlockHash:       []byte{4, 4, 4},
			SummaryValue:    []byte{5, 5, 5},
			RoundNumber:     2,
			SumOfEarnedFees: 1,
		}
		request := &certification.BlockCertificationRequest{
			SystemIdentifier: sysID1.ToSystemID(),
			NodeIdentifier:   "node1",
			InputRecord:      newIR,
			RootRoundNumber:  1,
		}
		require.NoError(t, request.Sign(signer))
		irChReq := &abtypes.IRChangeReq{
			SystemIdentifier: sysID1,
			CertReason:       abtypes.Quorum,
			Requests:         []*certification.BlockCertificationRequest{request},
		}
		data, err := ver.VerifyIRChangeReq(2, irChReq)
		require.Equal(t, newIR, data.IR)
		require.Equal(t, sysID1, data.SysID)
		require.NoError(t, err)
	})
}

func TestNewIRChangeReqVerifier(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: []byte{0, 0, 0, 1},
				T2Timeout:        2600,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	t.Run("partition store is nil", func(t *testing.T) {
		ver, err := NewIRChangeReqVerifier(&consensus.Parameters{}, nil, &MockState{})
		require.EqualError(t, err, "error partition store is nil")
		require.Nil(t, ver)
	})
	t.Run("state monitor is nil", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver, err := NewIRChangeReqVerifier(&consensus.Parameters{}, pInfo, nil)
		require.EqualError(t, err, "error state monitor is nil")
		require.Nil(t, ver)
	})
	t.Run("params is nil", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver, err := NewIRChangeReqVerifier(nil, pInfo, &MockState{})
		require.EqualError(t, err, "error consensus params is nil")
		require.Nil(t, ver)
	})
	t.Run("ok", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		ver, err := NewIRChangeReqVerifier(&consensus.Parameters{}, pInfo, &MockState{})
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
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: []byte{0, 0, 0, 1},
				T2Timeout:        2600,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	t.Run("state monitor is nil", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&consensus.Parameters{}, pInfo, nil)
		require.EqualError(t, err, "error state monitor is nil")
		require.Nil(t, tmoGen)
	})
	t.Run("partition store is nil", func(t *testing.T) {
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&consensus.Parameters{}, nil, &MockState{})
		require.EqualError(t, err, "error partition store is nil")
		require.Nil(t, tmoGen)
	})
	t.Run("params is nil", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		tmoGen, err := NewLucBasedT2TimeoutGenerator(nil, pInfo, &MockState{})
		require.EqualError(t, err, "error consensus params is nil")
		require.Nil(t, tmoGen)
	})
	t.Run("ok", func(t *testing.T) {
		pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
		require.NoError(t, err)
		tmoGen, err := NewLucBasedT2TimeoutGenerator(&consensus.Parameters{}, pInfo, &MockState{})
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
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID1.ToSystemID(),
				T2Timeout:        2500,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	pInfo, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
	require.NoError(t, err)
	state := &MockState{}
	tmoGen := &PartitionTimeoutGenerator{
		params:     &consensus.Parameters{BlockRate: 500 * time.Millisecond},
		state:      state,
		partitions: pInfo,
	}
	var tmos []types.SystemID32
	// last certified round is 1 then 11 - 1 = 10 we have not heard from partition in 10 rounds ~ at minimum 2500 ms not yet timeout
	tmos, err = tmoGen.GetT2Timeouts(11)
	require.NoError(t, err)
	require.Empty(t, tmos)
	// last certified round is 1 then 12 - 1 = 11 we have not heard from partition in 12 rounds ~ at minimum 2750 ms not yet timeout
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.EqualValues(t, []types.SystemID32{sysID1}, tmos)
	// mock sysID1 has pending change in pipeline - no timeout will be generated
	state.inProgress = append(state.inProgress, sysID1)
	state.irInProgress = irSysID1
	tmos, err = tmoGen.GetT2Timeouts(12)
	require.NoError(t, err)
	require.Empty(t, tmos)
}
