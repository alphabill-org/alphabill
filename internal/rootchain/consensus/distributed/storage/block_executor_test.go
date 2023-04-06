package storage

import (
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootchain/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

type (
	mockIRVerifier struct {
	}
)

var partitionID1 = []byte{0, 0, 0, 1}
var partitionID2 = []byte{0, 0, 0, 2}
var genesisInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{1, 1, 1, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func NewAlwaysTrueIRReqVerifier() *mockIRVerifier {
	return &mockIRVerifier{}
}

func (x *mockIRVerifier) VerifyIRChangeReq(_ uint64, irChReq *atomic_broadcast.IRChangeReqMsg) (*InputData, error) {
	return &InputData{SysID: irChReq.SystemIdentifier, IR: irChReq.Requests[0].InputRecord, Sdrh: []byte{0, 0, 0, 0, 1}}, nil
}

func generateBlockData(round uint64, req ...*atomic_broadcast.IRChangeReqMsg) *atomic_broadcast.BlockData {
	return &atomic_broadcast.BlockData{
		Author:    "test",
		Round:     round,
		Epoch:     0,
		Timestamp: 12,
		Payload:   &atomic_broadcast.Payload{Requests: req},
		Qc:        nil, // not important in this context
	}
}

func TestNewExecutedBlockFromGenesis(t *testing.T) {
	_, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, genesisInputRecord, partitionID1, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	hash := gocrypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	// partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	b := NewExecutedBlockFromGenesis(hash, rootGenesis.Partitions)
	require.Equal(t, b.HashAlgo, gocrypto.SHA256)
	data := b.CurrentIR.Find(partitionID1)
	require.Equal(t, rootGenesis.Partitions[0].SystemDescriptionRecord.SystemIdentifier, data.SysID)
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
	id := rootNode.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	hash := gocrypto.Hash(rootGenesis.Root.Consensus.HashAlgorithm)
	// partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	parent := NewExecutedBlockFromGenesis(hash, rootGenesis.Partitions)
	certReq := &certification.BlockCertificationRequest{
		SystemIdentifier: partitionID1,
		NodeIdentifier:   "1",
		InputRecord: &certificates.InputRecord{
			PreviousHash: []byte{1, 1, 1, 1},
			Hash:         []byte{2, 2, 2, 2},
			BlockHash:    []byte{3, 3, 3, 3},
			SummaryValue: []byte{4, 4, 4, 4},
		},
	}
	req := &atomic_broadcast.IRChangeReqMsg{
		SystemIdentifier: partitionID1,
		CertReason:       atomic_broadcast.IRChangeReqMsg_QUORUM,
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
	require.True(t, executedBlock.Changed.Find(partitionID1))
	require.Len(t, executedBlock.CurrentIR, 1)
	require.Equal(t, hash, executedBlock.HashAlgo)
	rootHash := []byte{0x65, 0xbd, 0xc1, 0x74, 0x63, 0xd, 0x8a, 0x72, 0xf8, 0xf2, 0x8e, 0x6f, 0x1d, 0xef, 0x90, 0x6e, 0xf7, 0xbb, 0xbb, 0xa6, 0x4, 0xb8, 0xd0, 0xf, 0xf, 0x46, 0x90, 0x82, 0x69, 0xf0, 0xa6, 0xec}
	require.Equal(t, rootHash, executedBlock.RootHash)
	require.Nil(t, executedBlock.Qc)
	require.Nil(t, executedBlock.CommitQc)
}

func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	block := &ExecutedBlock{
		BlockData: &atomic_broadcast.BlockData{
			Author:  "test",
			Round:   2,
			Payload: &atomic_broadcast.Payload{},
			Qc:      nil,
		},
		CurrentIR: InputRecords{
			{
				SysID: partitionID1,
				IR: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1, 1},
					Hash:         []byte{2, 2, 2, 2},
					BlockHash:    []byte{3, 3, 3, 3},
					SummaryValue: []byte{4, 4, 4, 4},
				},
				Sdrh: []byte{1, 2, 3, 4},
			},
			{
				SysID: partitionID2,
				IR: &certificates.InputRecord{
					PreviousHash: []byte{1, 1, 1, 1},
					Hash:         []byte{4, 4, 4, 4},
					BlockHash:    []byte{3, 3, 3, 3},
					SummaryValue: []byte{4, 4, 4, 4},
				},
				Sdrh: []byte{4, 5, 6, 7},
			},
		},
		Changed:  [][]byte{partitionID1, partitionID2},
		HashAlgo: gocrypto.SHA256,
		RootHash: []byte{0xD8, 0x57, 0xE9, 0x33, 0xD7, 0x57, 0x9B, 0x9D, 0xB8, 0x13, 0xBD, 0x97, 0x5A, 0x25, 0xCC, 0xBD, 0x7E,
			0x74, 0x10, 0xBC, 0x43, 0x3A, 0x4A, 0xD2, 0x5B, 0x4E, 0xF8, 0xB4, 0x89, 0x17, 0x79, 0x3D},
		Qc:       &atomic_broadcast.QuorumCert{},
		CommitQc: nil,
	}
	commitQc := &atomic_broadcast.QuorumCert{
		VoteInfo: &certificates.RootRoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: []byte{0, 0, 0, 0},
			RootHash:          make([]byte, gocrypto.SHA256.Size()),
		},
	}
	// root hash does not match
	certs, err := block.GenerateCertificates(commitQc)
	require.ErrorContains(t, err, "commit of block round 2 failed, root hash mismatch")
	require.Nil(t, certs)
	// make a correct qc
	commitQc = &atomic_broadcast.QuorumCert{
		VoteInfo: &certificates.RootRoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
			CurrentRootHash:   make([]byte, gocrypto.SHA256.Size()),
		},
		LedgerCommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: []byte{0, 0, 0, 0},
			RootHash: []byte{0xD8, 0x57, 0xE9, 0x33, 0xD7, 0x57, 0x9B, 0x9D, 0xB8, 0x13, 0xBD, 0x97, 0x5A, 0x25, 0xCC, 0xBD, 0x7E,
				0x74, 0x10, 0xBC, 0x43, 0x3A, 0x4A, 0xD2, 0x5B, 0x4E, 0xF8, 0xB4, 0x89, 0x17, 0x79, 0x3D,
			},
		},
	}
	certs, err = block.GenerateCertificates(commitQc)
	require.NoError(t, err)
	require.Len(t, certs, 2)
	require.NotNil(t, block.CommitQc)

}

/*
func TestExecutedBlock_GenerateCertificates(t *testing.T) {
	type fields struct {
		BlockData    *atomic_broadcast.BlockData
		CurrentIR    map[protocol.SystemIdentifier]*InputData
		Changed      map[protocol.SystemIdentifier]struct{}
		HashAlgo     crypto.Hash
		RootHash     []byte
		Qc           *atomic_broadcast.QuorumCert
		CommitQc     *atomic_broadcast.QuorumCert
		Certificates map[protocol.SystemIdentifier]*certificates.UnicityCertificate
	}
	type args struct {
		commitQc *atomic_broadcast.QuorumCert
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &ExecutedBlock{
				BlockData:    tt.fields.BlockData,
				CurrentIR:    tt.fields.CurrentIR,
				Changed:      tt.fields.Changed,
				HashAlgo:     tt.fields.HashAlgo,
				RootHash:     tt.fields.RootHash,
				Qc:           tt.fields.Qc,
				CommitQc:     tt.fields.CommitQc,
				Certificates: tt.fields.Certificates,
			}
			if err := x.GenerateCertificates(tt.args.commitQc); (err != nil) != tt.wantErr {
				t.Errorf("GenerateCertificates() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}


*/
