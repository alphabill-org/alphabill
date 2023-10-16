package storage

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

type (
	mockIRVerifier struct {
	}
)

var partitionID1 = types.SystemID32(1)
var partitionID2 = types.SystemID32(2)
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

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *abtypes.IRChangeReq) (*InputData, error) {
	return &InputData{SysID: irChReq.SystemIdentifier, IR: irChReq.Requests[0].InputRecord, Sdrh: []byte{0, 0, 0, 0, 1}}, nil
}

func generateBlockData(round uint64, req ...*abtypes.IRChangeReq) *abtypes.BlockData {
	return &abtypes.BlockData{
		Author:    "test",
		Round:     round,
		Epoch:     0,
		Timestamp: 12,
		Payload:   &abtypes.Payload{Requests: req},
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
	b := NewExecutedBlockFromGenesis(hash, rootGenesis.Partitions)
	require.Equal(t, b.HashAlgo, gocrypto.SHA256)
	data := b.CurrentIR.Find(partitionID1)
	require.Equal(t, rootGenesis.Partitions[0].SystemDescriptionRecord.SystemIdentifier, data.SysID.ToSystemID())
	require.Equal(t, rootGenesis.Partitions[0].Certificate.InputRecord, data.IR)
	require.Equal(t, rootGenesis.Partitions[0].Certificate.UnicityTreeCertificate.SystemDescriptionHash, data.Sdrh)
	require.Empty(t, b.Changed)
	require.Len(t, b.RootHash, 32)
	require.Len(t, b.Changed, 0)
	require.NotNil(t, b.Qc)
	require.NotNil(t, b.CommitQc)
	require.NotNil(t, b.BlockData)
	require.Equal(t, uint64(1), b.BlockData.Round)
	require.Equal(t, "genesis", b.BlockData.Author)
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
	parent := NewExecutedBlockFromGenesis(hash, rootGenesis.Partitions)
	certReq := &certification.BlockCertificationRequest{
		SystemIdentifier: partitionID1.ToSystemID(),
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
	req := &abtypes.IRChangeReq{
		SystemIdentifier: partitionID1,
		CertReason:       abtypes.Quorum,
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
	require.True(t, executedBlock.Changed.Contains(partitionID1))
	require.Len(t, executedBlock.CurrentIR, 1)
	require.Equal(t, hash, executedBlock.HashAlgo)
	rootHash := []byte{0xd0, 0xf7, 0xc6, 0x55, 0xcd, 0xa, 0xbc, 0x28, 0xc4, 0x26, 0x32, 0xaf, 0x6e, 0x29, 0x50, 0x69, 0x5e, 0xee, 0x55, 0x31, 0x6, 0xbd, 0xbd, 0x56, 0x7e, 0xa2, 0x77, 0xd7, 0xaa, 0x8b, 0xfc, 0x27}
	require.Equal(t, rootHash, executedBlock.RootHash)
	require.Nil(t, executedBlock.Qc)
	require.Nil(t, executedBlock.CommitQc)
}

func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	block := &ExecutedBlock{
		BlockData: &abtypes.BlockData{
			Author:  "test",
			Round:   2,
			Payload: &abtypes.Payload{},
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
		Changed:  []types.SystemID32{partitionID1, partitionID2},
		HashAlgo: gocrypto.SHA256,
		RootHash: []byte{0xF2, 0xE8, 0xBC, 0xC5, 0x71, 0x0D, 0x40, 0xAB, 0x42, 0xD5, 0x70, 0x57, 0x6F, 0x56, 0xA2, 0xF2,
			0x7E, 0xF6, 0x0F, 0xE9, 0x21, 0x25, 0x0A, 0x4B, 0x4C, 0xF5, 0xBC, 0xAC, 0xA3, 0x29, 0xBF, 0x32},
		Qc:       &abtypes.QuorumCert{},
		CommitQc: nil,
	}
	commitQc := &abtypes.QuorumCert{
		VoteInfo: &abtypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{
			PreviousHash: []byte{0, 0, 0, 0},
			Hash:         make([]byte, gocrypto.SHA256.Size()),
		},
	}
	// root hash does not match
	certs, err := block.GenerateCertificates(commitQc)
	require.ErrorContains(t, err, "commit of block round 2 failed, root hash mismatch")
	require.Nil(t, certs)
	// make a correct qc
	commitQc = &abtypes.QuorumCert{
		VoteInfo: &abtypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &types.UnicitySeal{
			PreviousHash: []byte{0, 0, 0, 0},
			Hash: []byte{0xF2, 0xE8, 0xBC, 0xC5, 0x71, 0x0D, 0x40, 0xAB, 0x42, 0xD5, 0x70, 0x57, 0x6F, 0x56, 0xA2, 0xF2,
				0x7E, 0xF6, 0x0F, 0xE9, 0x21, 0x25, 0x0A, 0x4B, 0x4C, 0xF5, 0xBC, 0xAC, 0xA3, 0x29, 0xBF, 0x32,
			},
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
	b = &ExecutedBlock{BlockData: &abtypes.BlockData{Round: 2}}
	require.Equal(t, uint64(2), b.GetRound())
}

func TestExecutedBlock_GetParentRound(t *testing.T) {
	var b *ExecutedBlock
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &abtypes.BlockData{}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &abtypes.BlockData{Qc: &abtypes.QuorumCert{}}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &abtypes.BlockData{Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{}}}}
	require.Equal(t, uint64(0), b.GetParentRound())
	b = &ExecutedBlock{BlockData: &abtypes.BlockData{Qc: &abtypes.QuorumCert{VoteInfo: &abtypes.RoundInfo{RoundNumber: 2}}}}
	require.Equal(t, uint64(2), b.GetParentRound())
}
