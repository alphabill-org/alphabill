package monolithic

import (
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/internal/rootvalidator/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/testutils"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

var sysID0 = []byte{0, 0, 0, 0}
var sysID1 = []byte{0, 0, 0, 1}
var sysID2 = []byte{0, 0, 0, 2}
var partitionID = []byte{0, 0xFF, 0, 1}
var partitionInputRecord = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
}

func readResult(ch <-chan certificates.UnicityCertificate, timeout time.Duration) (*certificates.UnicityCertificate, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("failed to read from channel")
		}
		return &result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func initConsensusManager(t *testing.T) (*ConsensusManager, *testutils.TestNode, []*testutils.TestNode, *genesis.RootGenesis) {
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.Peer.ID()
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitions, err := partition_store.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	// initiate state store
	stateStore := store.NewInMemStateStore()
	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
	}
	require.NoError(t, stateStore.Save(&store.RootState{
		LatestRound:    rootGenesis.GetRoundNumber(),
		Certificates:   certs,
		LatestRootHash: rootGenesis.GetRoundHash(),
	}))
	cm, err := NewMonolithicConsensusManager(rootNode.Peer.ID().String(), partitions, stateStore, rootNode.Signer)
	require.NoError(t, err)
	return cm, rootNode, partitionNodes, rootGenesis
}

func TestConsensusManager_checkT2Timeout(t *testing.T) {
	partitions, err := partition_store.NewPartitionStoreFromGenesis([]*genesis.GenesisPartitionRecord{
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysID0, T2Timeout: 2500}},
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysID1, T2Timeout: 2500}},
		{SystemDescriptionRecord: &genesis.SystemDescriptionRecord{SystemIdentifier: sysID2, T2Timeout: 2500}},
	})
	require.NoError(t, err)
	manager := &ConsensusManager{
		conf: &consensusConfig{
			hashAlgo:  gocrypto.SHA256,
			t3Timeout: 900 * time.Millisecond,
		},
		selfID:     "test",
		partitions: partitions,
		ir: map[p.SystemIdentifier]*certificates.InputRecord{
			p.SystemIdentifier(sysID0): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			p.SystemIdentifier(sysID1): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			p.SystemIdentifier(sysID2): {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
		},
		changes: map[p.SystemIdentifier]*certificates.InputRecord{},
	}
	// store mock state
	lastState := store.RootState{LatestRound: 4, LatestRootHash: []byte{0, 1}, Certificates: map[p.SystemIdentifier]*certificates.UnicityCertificate{
		p.SystemIdentifier(sysID0): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 1}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 3}}}, // no timeout (5 - 3) * 900 = 1800 ms
		p.SystemIdentifier(sysID1): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 2}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 2}}}, // timeout (5 - 2) * 900 = 2700 ms
		p.SystemIdentifier(sysID2): {
			InputRecord:            &certificates.InputRecord{Hash: []byte{1, 3}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal:            &certificates.UnicitySeal{RootRoundInfo: &certificates.RootRoundInfo{RoundNumber: 4}}}, // no timeout
	}}
	require.NoError(t, manager.checkT2Timeout(5, &lastState))
	// if round is 900ms then timeout of 2500 is reached in 3 * 900ms rounds, which is 2700ms
	require.Equal(t, 1, len(manager.changes))
	require.Contains(t, manager.changes, p.SystemIdentifier(sysID1))
}

func TestConsensusManager_NormalOperation(t *testing.T) {
	cm, rootNode, partitionNodes, rg := initConsensusManager(t)
	defer cm.Stop()
	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	// mock requests from partition node
	requests := make([]*certification.BlockCertificationRequest, 2)
	newIR := &certificates.InputRecord{
		PreviousHash: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	req := consensus.IRChangeRequest{
		SystemIdentifier: p.SystemIdentifier(partitionID),
		Reason:           consensus.Quorum,
		IR:               newIR,
		Requests:         requests}
	// submit IR change request from partition with quorum
	cm.RequestCertification() <- req
	// require, that certificates are received for partition ID
	result, err := readResult(cm.CertificationResult(), 1000*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, partitionInputRecord.Hash, result.InputRecord.PreviousHash)
	require.Equal(t, uint64(3), result.UnicitySeal.RootRoundInfo.RoundNumber)
	require.Equal(t, uint64(2), result.UnicitySeal.RootRoundInfo.ParentRoundNumber)
	require.NotNil(t, result.UnicitySeal.CommitInfo.RootHash)
	require.Equal(t, result.UnicitySeal.CommitInfo.RootHash, result.UnicitySeal.RootRoundInfo.CurrentRootHash)
	trustBase := map[string]crypto.Verifier{rootNode.Peer.ID().String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.IsValid(trustBase, gocrypto.SHA256, partitionID, sdrh))
}

// this will run long, cut timeouts or find a way to manipulate timeouts
func TestConsensusManager_PartitionTimeout(t *testing.T) {
	cm, rootNode, partitionNodes, rg := initConsensusManager(t)
	defer cm.Stop()
	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	// require, that repeat UC certificates are received for partition ID in 3 root rounds (partition timeout 2500 < 3 * 900)
	result, err := readResult(cm.CertificationResult(), 3000*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, partitionInputRecord, result.InputRecord)
	require.Equal(t, uint64(5), result.UnicitySeal.RootRoundInfo.RoundNumber)
	require.Equal(t, uint64(4), result.UnicitySeal.RootRoundInfo.ParentRoundNumber)
	require.NotNil(t, result.UnicitySeal.CommitInfo.RootHash)
	require.Equal(t, result.UnicitySeal.CommitInfo.RootHash, result.UnicitySeal.RootRoundInfo.CurrentRootHash)
	trustBase := map[string]crypto.Verifier{rootNode.Peer.ID().String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.IsValid(trustBase, gocrypto.SHA256, partitionID, sdrh))
}
