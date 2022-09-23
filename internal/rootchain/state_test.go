package rootchain

import (
	gocrypto "crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	rstore "github.com/alphabill-org/alphabill/internal/rootchain/store"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

var partition1ID = []byte{0, 0, 0, 1}
var partition2ID = []byte{0, 0, 0, 2}

var partition1IR = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 1, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
}

var partition2IR = &certificates.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 2, 1},
	BlockHash:    []byte{0, 0, 2, 2},
	SummaryValue: []byte{0, 0, 2, 3},
}

func Test_newStateFromGenesis_NotOk(t *testing.T) {
	signer, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	type args struct {
		g      *genesis.RootGenesis
		signer crypto.Signer
	}
	tests := []struct {
		name string
		args
		errStg string
	}{
		{
			name: "signer is nil",
			args: args{
				g:      &genesis.RootGenesis{},
				signer: nil,
			},
			errStg: "invalid root chain private key",
		},
		{
			name: "invalid genesis",
			args: args{
				g:      &genesis.RootGenesis{},
				signer: signer,
			},
			errStg: "invalid genesis",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewState(tt.args.g, "test", tt.args.signer, rstore.NewInMemoryRootChainStore())
			require.Nil(t, got)
			require.True(t, strings.Contains(err.Error(), tt.errStg))
		})
	}
}

func TestNewStateFromGenesis_Ok(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	_, _, partition2, _ := createPartitionRecord(t, partition2IR, partition2ID, 4)
	partitions := []*genesis.PartitionRecord{partition1, partition2}
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	rootGenesis, _, err := NewRootGenesis("test", rootSigner, rootPubKeyBytes, partitions)
	require.NoError(t, err)

	s1, err := NewState(rootGenesis, "test", rootSigner, rstore.NewInMemoryRootChainStore())
	require.NoError(t, err)

	s2 := createStateAndExecuteRound(t, partitions, rootSigner)
	require.NoError(t, err)
	//	require.Equal(t, s1.roundNumber, s2.roundNumber, 2)
	//	require.Equal(t, s1.previousRoundRootHash, s2.previousRoundRootHash)
	require.Equal(t, s1.partitionStore, s2.partitionStore)
	//	require.Equal(t, s1.latestUnicityCertificates, s2.latestUnicityCertificates)
	//	require.Equal(t, s1.inputRecords, s2.inputRecords)
	// Root genesis does not contain incoming requests,this is a runtime store
	require.Empty(t, s1.incomingRequests)
	// When state is constructed from partition genesis files, the incoming requests is initiated with partition requests
	require.NotEmpty(t, s2.incomingRequests)
	require.Equal(t, s1.hashAlgorithm, s2.hashAlgorithm)
	require.Equal(t, s1.signer, s2.signer)
	require.Equal(t, s1.verifiers, s2.verifiers)
}

func TestNewStateFromPartitionRecords_Ok(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootVerifier, err := rootSigner.Verifier()
	require.NoError(t, err)

	_, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	_, _, partition2, _ := createPartitionRecord(t, partition2IR, partition2ID, 4)

	s, err := NewStateFromPartitionRecords([]*genesis.PartitionRecord{partition1, partition2}, "test", rootSigner, gocrypto.SHA256, rstore.NewInMemoryRootChainStore())
	require.NoError(t, err)
	require.Equal(t, uint64(1), s.GetRoundNumber())
	// partition store checks
	require.Equal(t, 2, s.partitionStore.size())
	require.Equal(t, 5, s.partitionStore.nodeCount(p.SystemIdentifier(partition1ID)))
	require.Equal(t, 4, s.partitionStore.nodeCount(p.SystemIdentifier(partition2ID)))

	require.Equal(t, 0, s.store.UCCount())
	require.Equal(t, 0, len(s.store.GetAllIRs()))
	require.Equal(t, 2, len(s.incomingRequests))
	require.Equal(t, 5, len(s.incomingRequests[p.SystemIdentifier(partition1ID)].requests))
	require.Equal(t, 4, len(s.incomingRequests[p.SystemIdentifier(partition2ID)].requests))
	require.Equal(t, 1, len(s.incomingRequests[p.SystemIdentifier(partition1ID)].hashCounts))
	require.Equal(t, 1, len(s.incomingRequests[p.SystemIdentifier(partition2ID)].hashCounts))

	// prepare partition1 consensus
	require.True(t, s.checkConsensus(s.incomingRequests[p.SystemIdentifier(partition1ID)]))
	require.Equal(t, 1, len(s.store.GetAllIRs()))
	require.Equal(t, partition1IR, s.store.GetIR(p.SystemIdentifier(partition1ID)))

	// prepare partition1 consensus
	require.True(t, s.checkConsensus(s.incomingRequests[p.SystemIdentifier(partition2ID)]))
	require.Equal(t, 2, len(s.store.GetAllIRs()))
	require.Equal(t, partition2IR, s.store.GetIR(p.SystemIdentifier(partition2ID)))

	// create certificates
	_, err = s.CreateUnicityCertificates()
	require.NoError(t, err)
	require.Equal(t, 2, s.store.UCCount())
	p1UC := s.store.GetUC(p.SystemIdentifier(partition1ID))
	require.NotNil(t, p1UC)
	p2UC := s.store.GetUC(p.SystemIdentifier(partition2ID))
	require.NotNil(t, p2UC)
	require.NotEqual(t, p1UC, p2UC)
	require.Equal(t, p1UC.UnicitySeal, p2UC.UnicitySeal)
	p1Hash := partition1.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	p2Hash := partition2.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	verifiers := map[string]crypto.Verifier{"test": rootVerifier}
	require.NoError(t, p1UC.IsValid(verifiers, s.hashAlgorithm, partition1ID, p1Hash))
	require.NoError(t, p2UC.IsValid(verifiers, s.hashAlgorithm, partition2ID, p2Hash))

	// verify State after the round
	require.Equal(t, uint64(2), s.GetRoundNumber())
	require.Equal(t, p1UC.UnicitySeal.Hash, s.store.GetPreviousRoundRootHash())
	require.Equal(t, 2, len(s.incomingRequests))
	require.Equal(t, 0, len(s.incomingRequests[p.SystemIdentifier(partition1ID)].requests))
	require.Equal(t, 0, len(s.incomingRequests[p.SystemIdentifier(partition2ID)].requests))
	require.Equal(t, 0, len(s.incomingRequests[p.SystemIdentifier(partition1ID)].hashCounts))
	require.Equal(t, 0, len(s.incomingRequests[p.SystemIdentifier(partition2ID)].hashCounts))
}

func TestHandleInputRequestEvent_OlderUnicityCertificate(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	s.CopyOldInputRecords(p.SystemIdentifier(partition1ID))
	_, err := s.CreateUnicityCertificates()
	require.NoError(t, err)

	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.ErrorContains(t, err, "old request")
	require.NotNil(t, receivedUc)
	require.Greater(t, receivedUc.UnicitySeal.RootChainRoundNumber, req.RootRoundNumber)

}

func TestHandleInputRequestEvent_ExtendingUnknownState(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.InputRecord.PreviousHash = []byte{1, 1, 1}
	err := req.Sign(signers[0])
	require.NoError(t, err)

	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.ErrorContains(t, err, "request extends unknown state")
	require.NotNil(t, receivedUc)
	require.NotEqual(t, req.InputRecord.PreviousHash, receivedUc.InputRecord.PreviousHash)
}

func TestHandleInputRequestEvent_Duplicated(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.Nil(t, receivedUc)
	require.Nil(t, err)
	receivedUc, err = s.HandleBlockCertificationRequest(req)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "duplicated request")
}

func TestHandleInputRequestEvent_UnknownSystemIdentifier(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.SystemIdentifier = []byte{0, 0, 0xAA, 0xAA}

	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "unknown SystemIdentifier")
}

func TestHandleInputRequestEvent_PartitionHasNewerUC(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.RootRoundNumber = 101
	require.NoError(t, req.Sign(signers[0]))
	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.NotNil(t, receivedUc)
	require.ErrorContains(t, err, "partition has never unicity certificate")
	require.Less(t, receivedUc.UnicitySeal.RootChainRoundNumber, req.RootRoundNumber)
}

func TestHandleInputRequestEvent_UnknownNodeID(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.NodeIdentifier = "unknown"
	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "unknown node identifier")
}

func TestHandleInputRequestEvent_RequestMissing(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	_, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	receivedUc, err := s.HandleBlockCertificationRequest(nil)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "input record is ni")
}

func TestHandleInputRequestEvent_InvalidSignature(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	invalidReq := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	invalidReq.InputRecord.Hash = make([]byte, 32)
	receivedUc, err := s.HandleBlockCertificationRequest(invalidReq)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "verification failed")
}

func TestHandleInputRequestEvent_DuplicateWithDifferentHash(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])

	receivedUc, err := s.HandleBlockCertificationRequest(req)
	require.Nil(t, receivedUc)
	require.Nil(t, err)
	invalidReq := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	invalidReq.InputRecord.Hash = make([]byte, 32)
	require.NoError(t, invalidReq.Sign(signers[0]))
	receivedUc, err = s.HandleBlockCertificationRequest(invalidReq)
	require.Nil(t, receivedUc)
	require.ErrorContains(t, err, "equivocating request with different hash")
}

func TestHandleInputRequestEvent_ConsensusNotPossible(t *testing.T) {
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	signers, _, partition1, _ := createPartitionRecord(t, partition1IR, partition1ID, 3)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateBlockCertificationRequest(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.InputRecord.Hash = []byte{0, 0, 0}
	require.NoError(t, req.Sign(signers[0]))
	uc, err := s.HandleBlockCertificationRequest(req)
	require.NoError(t, err)
	require.Nil(t, uc)
	r1 := CreateBlockCertificationRequest(partition1.Validators[1].NodeIdentifier, s, signers[0])
	r1.InputRecord.Hash = []byte{1, 1, 1}
	require.NoError(t, r1.Sign(signers[1]))
	uc, err = s.HandleBlockCertificationRequest(r1)
	require.NoError(t, err)
	require.Nil(t, uc)
	r2 := CreateBlockCertificationRequest(partition1.Validators[2].NodeIdentifier, s, signers[0])
	r2.InputRecord.Hash = []byte{2, 2, 2}
	require.NoError(t, r2.Sign(signers[2]))
	uc, err = s.HandleBlockCertificationRequest(r2)
	require.NoError(t, err)
	require.Nil(t, uc)
	sysIds, err := s.CreateUnicityCertificates()
	require.NoError(t, err)
	require.Equal(t, 1, len(sysIds))
	luc := s.store.GetUC(sysIds[0])
	require.NotEqual(t, req.InputRecord.Hash, luc.InputRecord.Hash)
	require.NotEqual(t, r1.InputRecord.Hash, luc.InputRecord.Hash)
	require.NotEqual(t, r2.InputRecord.Hash, luc.InputRecord.Hash)
}

func createStateAndExecuteRound(t *testing.T, partitions []*genesis.PartitionRecord, rootSigner crypto.Signer) *State {
	t.Helper()
	s2, err := NewStateFromPartitionRecords(partitions, "test", rootSigner, gocrypto.SHA256, rstore.NewInMemoryRootChainStore())
	require.NoError(t, err)
	for _, pt := range partitions {
		id := p.SystemIdentifier(pt.SystemDescriptionRecord.SystemIdentifier)
		s2.checkConsensus(s2.incomingRequests[id])
	}
	_, err = s2.CreateUnicityCertificates()
	require.NoError(t, err)
	return s2
}

func createPartitionRecord(t *testing.T, ir *certificates.InputRecord, systemID []byte, nrOfValidators int) (signers []crypto.Signer, verifiers []crypto.Verifier, record *genesis.PartitionRecord, partitionPeers []*network.Peer) {
	record = &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemID,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{},
	}

	for i := 0; i < nrOfValidators; i++ {
		partitionNodePeer := peer.CreatePeer(t)
		partition1Signer, verifier := testsig.CreateSignerAndVerifier(t)

		encPubKey, err := partitionNodePeer.PublicKey()
		require.NoError(t, err)
		rawEncPubKey, err := encPubKey.Raw()
		require.NoError(t, err)

		rawSigningPubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)

		req := &certification.BlockCertificationRequest{
			SystemIdentifier: systemID,
			NodeIdentifier:   partitionNodePeer.ID().Pretty(),
			RootRoundNumber:  1,
			InputRecord:      ir,
		}
		err = req.Sign(partition1Signer)
		require.NoError(t, err)

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			NodeIdentifier:            partitionNodePeer.ID().Pretty(),
			SigningPublicKey:          rawSigningPubKey,
			EncryptionPublicKey:       rawEncPubKey,
			BlockCertificationRequest: req,
		})

		signers = append(signers, partition1Signer)
		verifiers = append(verifiers, verifier)
		partitionPeers = append(partitionPeers, partitionNodePeer)
	}
	return
}

func CreateBlockCertificationRequest(nodeIdentifier string, s *State, signers crypto.Signer) *certification.BlockCertificationRequest {
	req := &certification.BlockCertificationRequest{
		SystemIdentifier: partition1ID,
		NodeIdentifier:   nodeIdentifier,
		RootRoundNumber:  s.GetRoundNumber() - 1,
		InputRecord: &certificates.InputRecord{
			PreviousHash: partition1IR.Hash,
			Hash:         []byte{0, 1, 0, 1},
			BlockHash:    []byte{0, 2, 0, 1},
			SummaryValue: []byte{0, 0, 0, 1},
		},
	}
	req.Sign(signers)
	return req
}
