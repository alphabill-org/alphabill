package storage

import (
	gocrypto "crypto"
	"fmt"
	"slices"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

const partitionID1 types.SystemID = 1
const partitionID2 types.SystemID = 2

var genesisInputRecord = &types.InputRecord{
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
	return &InputData{SysID: irChReq.SystemIdentifier, IR: irChReq.Requests[0].InputRecord, Sdrh: []byte{0, 0, 0, 0, 1}}, nil
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
	require.Equal(t, rootGenesis.Partitions[0].PartitionDescription.SystemIdentifier, data.SysID)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.InputRecord, data.IR)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.UnicityTreeCertificate.PartitionDescriptionHash, data.Sdrh)
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
		SystemIdentifier: partitionID1,
		NodeIdentifier:   "1",
		InputRecord: &types.InputRecord{
			PreviousHash:    []byte{1, 1, 1, 1},
			Hash:            []byte{2, 2, 2, 2},
			BlockHash:       []byte{3, 3, 3, 3},
			SummaryValue:    []byte{4, 4, 4, 4},
			RoundNumber:     4,
			SumOfEarnedFees: 3,
		},
	}
	req := &drctypes.IRChangeReq{
		SystemIdentifier: partitionID1,
		CertReason:       drctypes.Quorum,
		Requests:         []*certification.BlockCertificationRequest{certReq},
	}
	newBlock := generateBlockData(genesis.RootRound+1, req)
	reqVer := NewAlwaysTrueIRReqVerifier()
	executedBlock, err := NewExecutedBlock(hash, newBlock, parent, reqVer)
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
	require.EqualValues(t, "E3C6967082C5D3E3FB0B40683F0491580B35B71E3727B9CC3916E615842739D1", fmt.Sprintf("%X", executedBlock.RootHash))
	// block has not got QC nor commit QC yet
	require.Nil(t, executedBlock.Qc)
	require.Nil(t, executedBlock.CommitQc)
}

func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	block := &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:  "test",
			Round:   2,
			Payload: &drctypes.Payload{},
			Qc:      nil,
		},
		CurrentIR: InputRecords{
			{
				SysID: partitionID1,
				IR: &types.InputRecord{
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{2, 2, 2, 2},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 4,
				},
				Sdrh: []byte{1, 2, 3, 4},
			},
			{
				SysID: partitionID2,
				IR: &types.InputRecord{
					PreviousHash:    []byte{1, 1, 1, 1},
					Hash:            []byte{4, 4, 4, 4},
					BlockHash:       []byte{3, 3, 3, 3},
					SummaryValue:    []byte{4, 4, 4, 4},
					RoundNumber:     3,
					SumOfEarnedFees: 6,
				},
				Sdrh: []byte{4, 5, 6, 7},
			},
		},
		Changed:  []types.SystemID{partitionID1, partitionID2},
		HashAlgo: gocrypto.SHA256,
		RootHash: []byte{0x9b, 0x98, 0xf9, 0x3b, 0xcf, 0x8d, 0xd8, 0x74, 0x88, 0xe6, 0x2c, 0xd5, 0x2f, 0x15, 0x10, 0xa5, 0xc6, 0xd1, 0xad, 0xc, 0xc3, 0x8f, 0xf8, 0xca, 0x87, 0x9b, 0x85, 0x66, 0x99, 0x6b, 0xef, 0xa3},
	}
	commitQc := &drctypes.QuorumCert{
		VoteInfo: &drctypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
			seal.PreviousHash = []byte{0, 0, 0, 0}
			seal.Hash = make([]byte, gocrypto.SHA256.Size())
		}),
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
		LedgerCommitInfo: types.NewUnicitySealV1(func(seal *types.UnicitySeal) {
			seal.PreviousHash = []byte{0, 0, 0, 0}
			seal.Hash = []byte{0x9b, 0x98, 0xf9, 0x3b, 0xcf, 0x8d, 0xd8, 0x74, 0x88, 0xe6, 0x2c, 0xd5, 0x2f, 0x15, 0x10, 0xa5, 0xc6, 0xd1, 0xad, 0xc, 0xc3, 0x8f, 0xf8, 0xca, 0x87, 0x9b, 0x85, 0x66, 0x99, 0x6b, 0xef, 0xa3}
		}),
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
