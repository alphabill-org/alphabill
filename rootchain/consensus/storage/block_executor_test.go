package storage

import (
	"crypto"
	"encoding/hex"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

const partitionID1 types.PartitionID = 1
const partitionID2 types.PartitionID = 2

var genesisInputRecord = &types.InputRecord{
	Version:      1,
	PreviousHash: make([]byte, 32),
	Hash:         []byte{1, 1, 1, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
	Timestamp:    types.NewTimestamp(),
}

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

type mockIRVerifier struct{}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *drctypes.IRChangeReq) (*InputData, error) {
	return &InputData{Partition: irChReq.Partition, IR: irChReq.Requests[0].InputRecord, ShardConfHash: []byte{0, 0, 0, 0, 1}}, nil
}

func TestNewExecutedBlockFromGenesis(t *testing.T) {
	peers, nodes := testutils.CreateTestNodes(t, genesisInputRecord, partitionID1, 3)
	rootNode := testutils.NewTestNode(t)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, nodes)
	require.NoError(t, err)
	hash := crypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	var varNodes []*types.NodeInfo
	for _, peer := range peers {
		varNodes = append(varNodes, &types.NodeInfo{
			NodeID: peer.PeerConf.ID.String(),
			SigKey: peer.PeerConf.KeyPair.PublicKey,
			Stake:  1,
		})
	}
	b, err := newGenesisBlock(hash, rootGenesis.Partitions)
	require.NoError(t, err)
	require.Equal(t, b.HashAlgo, crypto.SHA256)
	data := b.CurrentIR.Find(partitionID1)
	require.Equal(t, rootGenesis.Partitions[0].PartitionDescription.PartitionID, data.Partition)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.InputRecord, data.IR)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.UnicityTreeCertificate.PDRHash, data.ShardConfHash)
	require.Empty(t, b.Changed)
	require.Len(t, b.RootHash, 32)
	require.Len(t, b.Changed, 0)
	require.NotNil(t, b.BlockData)
	require.Equal(t, uint64(1), b.BlockData.Round)
	require.Equal(t, "genesis", b.BlockData.Author)
	require.NotNil(t, b.BlockData.Qc)
	require.NoError(t, b.BlockData.Qc.IsValid())
	require.NotNil(t, b.Qc)
	require.NoError(t, b.Qc.IsValid())
	require.NotNil(t, b.CommitQc)
	require.NoError(t, b.CommitQc.IsValid())
}

func TestExecutedBlock_Extend(t *testing.T) {
	peers, nodes := testutils.CreateTestNodes(t, genesisInputRecord, partitionID1, 3)
	rootNode := testutils.NewTestNode(t)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, nodes)
	require.NoError(t, err)
	hash := crypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	var varNodes []*types.NodeInfo
	for _, peer := range peers {
		varNodes = append(varNodes, &types.NodeInfo{
			NodeID: peer.PeerConf.ID.String(),
			SigKey: peer.PeerConf.KeyPair.PublicKey,
		})
	}
	orchestration := mockOrchestration{
		shardEpoch: func(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error) {
			return genesisInputRecord.Epoch, nil
		},
		shardConfig: func(partition types.PartitionID, shard types.ShardID, epoch uint64) (*partitions.ValidatorAssignmentRecord, error) {
			return &partitions.ValidatorAssignmentRecord{
				PartitionID: partition,
				ShardID:     shard,
				EpochNumber: epoch,
				Nodes:       varNodes,
			}, nil
		},
	}
	parent, err := newGenesisBlock(hash, rootGenesis.Partitions)
	require.NoError(t, err)
	certReq := &certification.BlockCertificationRequest{
		PartitionID: partitionID1,
		NodeID:      "1",
		InputRecord: &types.InputRecord{
			Version:         1,
			PreviousHash:    []byte{1, 1, 1, 1},
			Hash:            []byte{2, 2, 2, 2},
			BlockHash:       []byte{3, 3, 3, 3},
			SummaryValue:    []byte{4, 4, 4, 4},
			RoundNumber:     4,
			SumOfEarnedFees: 3,
		},
	}
	newBlock := &drctypes.BlockData{
		Author:    "test",
		Round:     genesis.RootRound + 1,
		Epoch:     0,
		Timestamp: 12,
		Payload: &drctypes.Payload{
			Requests: []*drctypes.IRChangeReq{{
				Partition:  partitionID1,
				CertReason: drctypes.Quorum,
				Requests:   []*certification.BlockCertificationRequest{certReq},
			}},
		},
		Qc: nil, // not important in this context
	}
	reqVer := NewAlwaysTrueIRReqVerifier()
	executedBlock, err := parent.Extend(hash, newBlock, reqVer, orchestration)
	require.NoError(t, err)
	require.Equal(t, "test", executedBlock.BlockData.Author)
	require.Equal(t, genesis.RootRound+1, executedBlock.BlockData.Round)
	require.Equal(t, certReq, executedBlock.BlockData.Payload.Requests[0].Requests[0])
	require.Len(t, executedBlock.Changed, 1)
	require.Contains(t, executedBlock.Changed, types.PartitionShardID{PartitionID: partitionID1, ShardID: types.ShardID{}.Key()})
	require.Len(t, executedBlock.CurrentIR, 1)
	require.Equal(t, certReq.InputRecord, executedBlock.CurrentIR.Find(partitionID1).IR)
	// parent remains unchanged
	require.Equal(t, genesisInputRecord, parent.CurrentIR.Find(partitionID1).IR)
	require.Equal(t, hash, executedBlock.HashAlgo)
	// can't compare against hardcoded hash as fee hash and leader id change on each run (we generate partitionRecord)
	//require.EqualValues(t, "99AD3740E3CFC07EC1C1C04ED60D930BC3E2DC01AD5B3E8631C119C50EAF4520", fmt.Sprintf("%X", executedBlock.RootHash))
	// block has not got QC nor commit QC yet
	require.Nil(t, executedBlock.Qc)
	require.Nil(t, executedBlock.CommitQc)
}

func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	rh, err := hex.DecodeString("B8E278ACF8DB0FADF5AA7A2C079CB888DEB38303020218A003F42B32E44F1F9E")
	require.NoError(t, err)
	block := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:  "test",
			Round:   2,
			Payload: &drctypes.Payload{},
			Qc:      nil,
		},
		CurrentIR: InputRecords{
			{
				Partition: partitionID1,
				IR: &types.InputRecord{
					Version:         1,
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{2, 2, 2, 2},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 4,
					Timestamp:       20241113,
				},
				ShardConfHash: []byte{1, 2, 3, 4},
			},
			{
				Partition: partitionID2,
				IR: &types.InputRecord{
					Version:         1,
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{4, 4, 4, 4},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 6,
					Timestamp:       20241113,
				},
				ShardConfHash: []byte{4, 5, 6, 7},
			},
		},
		ShardInfo: shardStates{
			types.PartitionShardID{PartitionID: partitionID1, ShardID: types.ShardID{}.Key()}: &ShardInfo{},
			types.PartitionShardID{PartitionID: partitionID2, ShardID: types.ShardID{}.Key()}: &ShardInfo{},
		},
		Changed: map[types.PartitionShardID]struct{}{
			{PartitionID: partitionID1, ShardID: types.ShardID{}.Key()}: {},
			{PartitionID: partitionID2, ShardID: types.ShardID{}.Key()}: {},
		},
		HashAlgo: crypto.SHA256,
		RootHash: rh,
	}

	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, crypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: []byte{0, 0, 0, 0},
			Hash:         make([]byte, crypto.SHA256.Size()),
		},
	}
	// root hash does not match
	certs, err := block.GenerateCertificates(commitQc)
	require.ErrorContains(t, err, "root hash does not match hash in commit QC")
	require.Nil(t, certs)
	// make a correct qc
	commitQc = &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, crypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{
			Version:      1,
			PreviousHash: []byte{0, 0, 0, 0},
			Hash:         rh,
		},
	}
	certs, err = block.GenerateCertificates(commitQc)
	require.NoError(t, err)
	require.Len(t, certs, 2)
	si, ok := block.ShardInfo[types.PartitionShardID{PartitionID: partitionID1, ShardID: types.ShardID{}.Key()}]
	require.True(t, ok)
	require.NotNil(t, si.LastCR)
}

func TestExecutedBlock_GetRound(t *testing.T) {
	var b *ExecutedBlock
	require.Equal(t, uint64(0), b.GetRound())
	b = &ExecutedBlock{BlockData: nil}
	require.Equal(t, uint64(0), b.GetRound())
	b = &ExecutedBlock{BlockData: &drctypes.BlockData{Round: 2}}
	require.Equal(t, uint64(2), b.GetRound())
}

func TestExecutedBlock_GetParentRound(t *testing.T) {
	var b *ExecutedBlock
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &drctypes.BlockData{}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &drctypes.BlockData{Qc: &drctypes.QuorumCert{}}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &drctypes.BlockData{Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{}}}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &drctypes.BlockData{Qc: &drctypes.QuorumCert{VoteInfo: &drctypes.RoundInfo{RoundNumber: 2}}}}
	require.Equal(t, uint64(2), b.GetParentRound())
}

func Test_ExecutedBlock_serialization(t *testing.T) {
	t.Run("Changed set", func(t *testing.T) {
		// empty set
		// we init the Changed manually to non-nil value as require.EqualValues
		// considers nil and empty map as different. In code the ExecutedBlock
		// values are constructed via constructors which init the Changed field.
		b1 := ExecutedBlock{Changed: ShardSet{}}
		buf, err := types.Cbor.Marshal(b1)
		require.NoError(t, err)

		var b2 ExecutedBlock
		require.NoError(t, types.Cbor.Unmarshal(buf, &b2))
		require.EqualValues(t, b1.Changed, b2.Changed)

		// set with one item
		b1.Changed = map[types.PartitionShardID]struct{}{{PartitionID: 1, ShardID: types.ShardID{}.Key()}: {}}
		buf, err = types.Cbor.Marshal(b1)
		require.NoError(t, err)

		require.NoError(t, types.Cbor.Unmarshal(buf, &b2))
		require.Equal(t, b1.Changed, b2.Changed)
	})

	t.Run("ShardInfo", func(t *testing.T) {
		// empty map
		b1 := ExecutedBlock{ShardInfo: shardStates{}}
		buf, err := types.Cbor.Marshal(b1)
		require.NoError(t, err)

		var b2 ExecutedBlock
		require.NoError(t, types.Cbor.Unmarshal(buf, &b2))
		require.EqualValues(t, b1.ShardInfo, b2.ShardInfo)

		// non-empty map
		si := ShardInfo{
			RootHash:      []byte{3, 3, 3},
			PrevEpochStat: []byte{0x43, 4, 4, 4}, // array(3)
			PrevEpochFees: []byte{0x43, 5, 5, 5},
			Fees:          map[string]uint64{"A": 10},
			LastCR: &certification.CertificationResponse{
				Partition: 9,
				Shard:     types.ShardID{},
				Technical: certification.TechnicalRecord{
					Round:  2,
					Epoch:  3,
					Leader: "ldr",
				},
				UC: types.UnicityCertificate{
					Version: 1,
				},
			},
		}
		psKey := types.PartitionShardID{PartitionID: si.LastCR.Partition, ShardID: si.LastCR.Shard.Key()}
		b1.ShardInfo[psKey] = &si
		buf, err = types.Cbor.Marshal(b1)
		require.NoError(t, err)

		require.NoError(t, types.Cbor.Unmarshal(buf, &b2))
		require.Equal(t, b1.ShardInfo, b2.ShardInfo)
		require.Equal(t, &si, b2.ShardInfo[psKey])
	})
}
