package storage

import (
	gocrypto "crypto"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

const partitionID1 types.PartitionID = 1
const partitionID2 types.PartitionID = 2

var genesisInputRecord = &types.InputRecord{Version: 1,
	PreviousHash: make([]byte, 32),
	Hash:         []byte{1, 1, 1, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

type mockIRVerifier struct{}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *drctypes.IRChangeReq) (*InputData, error) {
	return &InputData{Partition: irChReq.Partition, IR: irChReq.Requests[0].InputRecord, PDRHash: []byte{0, 0, 0, 0, 1}}, nil
}

func generateBlockData(round uint64, req ...*drctypes.IRChangeReq) *drctypes.BlockData {
	return &drctypes.BlockData{
		Author:    "test",
		Round:     round,
		Epoch:     0,
		Timestamp: 12,
		Payload:   &drctypes.Payload{Requests: req},
		Qc:        nil, // not important in this context
	}
}

func TestNewExecutedBlockFromGenesis(t *testing.T) {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, genesisInputRecord, partitionID1, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	hash := gocrypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	// partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	b := NewGenesisBlock(hash, rootGenesis.Partitions)
	require.Equal(t, b.HashAlgo, gocrypto.SHA256)
	data := b.CurrentIR.Find(partitionID1)
	require.Equal(t, rootGenesis.Partitions[0].PartitionDescription.PartitionIdentifier, data.Partition)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.InputRecord, data.IR)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.UnicityTreeCertificate.PartitionDescriptionHash, data.PDRHash)
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

func TestExecutedBlock(t *testing.T) {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, genesisInputRecord, partitionID1, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	hash := gocrypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	// partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	parent := NewGenesisBlock(hash, rootGenesis.Partitions)
	certReq := &certification.BlockCertificationRequest{
		Partition:      partitionID1,
		NodeIdentifier: "1",
		InputRecord: &types.InputRecord{Version: 1,
			PreviousHash:    []byte{1, 1, 1, 1},
			Hash:            []byte{2, 2, 2, 2},
			BlockHash:       []byte{3, 3, 3, 3},
			SummaryValue:    []byte{4, 4, 4, 4},
			RoundNumber:     4,
			SumOfEarnedFees: 3,
		},
	}
	req := &drctypes.IRChangeReq{
		Partition:  partitionID1,
		CertReason: drctypes.Quorum,
		Requests:   []*certification.BlockCertificationRequest{certReq},
	}
	newBlock := generateBlockData(genesis.RootRound+1, req)
	reqVer := NewAlwaysTrueIRReqVerifier()
	getTRFunc := func(types.PartitionID, types.ShardID, *certification.BlockCertificationRequest) (certification.TechnicalRecord, error) {
		return certification.TechnicalRecord{}, nil
	}
	executedBlock, err := NewExecutedBlock(hash, newBlock, parent, reqVer, getTRFunc)
	require.NoError(t, err)
	require.Equal(t, "test", executedBlock.BlockData.Author)
	require.Equal(t, genesis.RootRound+1, executedBlock.BlockData.Round)
	require.Equal(t, certReq, executedBlock.BlockData.Payload.Requests[0].Requests[0])
	require.Len(t, executedBlock.Changed, 1)
	require.True(t, slices.Contains(executedBlock.Changed, partitionID1))
	require.Len(t, executedBlock.CurrentIR, 1)
	require.Equal(t, certReq.InputRecord, executedBlock.CurrentIR.Find(partitionID1).IR)
	// parent remains unchanged
	require.Equal(t, genesisInputRecord, parent.CurrentIR.Find(partitionID1).IR)
	require.Equal(t, hash, executedBlock.HashAlgo)
	require.EqualValues(t, "89D3869C9284F932817E48CA5CEA7979000292E0072D3D5264D5ABEA2054E912", fmt.Sprintf("%X", executedBlock.RootHash))
	// block has not got QC nor commit QC yet
	require.Nil(t, executedBlock.Qc)
	require.Nil(t, executedBlock.CommitQc)
}

func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	rh, err := hex.DecodeString("DDA4C864E4365DDB45B6555D3815BFAC9227C01887540DD653169CBA3D7BCB4F")
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
				IR: &types.InputRecord{Version: 1,
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{2, 2, 2, 2},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 4,
				},
				PDRHash: []byte{1, 2, 3, 4},
			},
			{
				Partition: partitionID2,
				IR: &types.InputRecord{Version: 1,
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{4, 4, 4, 4},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 6,
				},
				PDRHash: []byte{4, 5, 6, 7},
			},
		},
		Changed:  []types.PartitionID{partitionID1, partitionID2},
		HashAlgo: gocrypto.SHA256,
		RootHash: rh,
	}

	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			PreviousHash: []byte{0, 0, 0, 0},
			Hash:         make([]byte, gocrypto.SHA256.Size()),
		},
	}
	// root hash does not match
	certs, err := block.GenerateCertificates(commitQc)
	require.ErrorContains(t, err, "commit of block round 2 failed, root hash mismatch")
	require.Nil(t, certs)
	// make a correct qc
	commitQc = &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{Version: 1,
			PreviousHash: []byte{0, 0, 0, 0},
			Hash:         rh,
		},
	}
	certs, err = block.GenerateCertificates(commitQc)
	require.NoError(t, err)
	require.Len(t, certs, 2)
	require.NotNil(t, block.CommitQc)
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
