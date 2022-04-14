package rootchain

import (
	gocrypto "crypto"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
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
			got, err := NewStateFromGenesis(tt.args.g, tt.args.signer)
			require.Nil(t, got)
			require.True(t, strings.Contains(err.Error(), tt.errStg))
		})
	}
}

func TestNewStateFromGenesis_Ok(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	_, _, partition2 := createPartitionRecord(t, partition2IR, partition2ID, 4)
	partitions := []*genesis.PartitionRecord{partition1, partition2}
	rootGenesis, _, err := NewGenesis(partitions, rootSigner)
	require.NoError(t, err)

	s1, err := NewStateFromGenesis(rootGenesis, rootSigner)
	require.NoError(t, err)

	s2 := createStateAndExecuteRound(t, partitions, rootSigner)
	require.NoError(t, err)
	require.Equal(t, s1, s2)
}

func TestNewStateFromPartitionRecords_Ok(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	rootVerifier, err := rootSigner.Verifier()
	require.NoError(t, err)

	_, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	_, _, partition2 := createPartitionRecord(t, partition2IR, partition2ID, 4)

	s, err := NewStateFromPartitionRecords([]*genesis.PartitionRecord{partition1, partition2}, rootSigner, gocrypto.SHA256)
	require.NoError(t, err)
	require.Equal(t, uint64(1), s.roundNumber)
	// partition store checks
	require.Equal(t, 2, s.partitionStore.size())
	require.Equal(t, 5, s.partitionStore.nodeCount(string(partition1ID)))
	require.Equal(t, 4, s.partitionStore.nodeCount(string(partition2ID)))

	require.Equal(t, 0, s.latestUnicityCertificates.size())
	require.Equal(t, 0, len(s.inputRecords))
	require.Equal(t, 2, len(s.incomingRequests))
	require.Equal(t, 5, len(s.incomingRequests[string(partition1ID)].requests))
	require.Equal(t, 4, len(s.incomingRequests[string(partition2ID)].requests))
	require.Equal(t, 1, len(s.incomingRequests[string(partition1ID)].hashCounts))
	require.Equal(t, 1, len(s.incomingRequests[string(partition2ID)].hashCounts))

	// prepare partition1 consensus
	require.True(t, s.checkConsensus(string(partition1ID)))
	require.Equal(t, 1, len(s.inputRecords))
	require.Equal(t, partition1IR, s.inputRecords[string(partition1ID)])

	// prepare partition1 consensus
	require.True(t, s.checkConsensus(string(partition2ID)))
	require.Equal(t, 2, len(s.inputRecords))
	require.Equal(t, partition2IR, s.inputRecords[string(partition2ID)])

	// create certificates
	_, err = s.CreateUnicityCertificates()
	require.NoError(t, err)
	require.Equal(t, 2, s.latestUnicityCertificates.size())
	p1UC := s.latestUnicityCertificates.get(string(partition1ID))
	require.NotNil(t, p1UC)
	p2UC := s.latestUnicityCertificates.get(string(partition2ID))
	require.NotNil(t, p2UC)
	require.NotEqual(t, p1UC, p2UC)
	require.Equal(t, p1UC.UnicitySeal, p2UC.UnicitySeal)
	p1Hash := partition1.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	p2Hash := partition2.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	require.NoError(t, p1UC.IsValid(rootVerifier, s.hashAlgorithm, partition1ID, p1Hash))
	require.NoError(t, p2UC.IsValid(rootVerifier, s.hashAlgorithm, partition2ID, p2Hash))

	// verify State after the round
	require.Equal(t, uint64(2), s.roundNumber)
	require.Equal(t, p1UC.UnicitySeal.Hash, s.previousRoundRootHash)
	require.Equal(t, 2, len(s.incomingRequests))
	require.Equal(t, 0, len(s.incomingRequests[string(partition1ID)].requests))
	require.Equal(t, 0, len(s.incomingRequests[string(partition2ID)].requests))
	require.Equal(t, 0, len(s.incomingRequests[string(partition1ID)].hashCounts))
	require.Equal(t, 0, len(s.incomingRequests[string(partition2ID)].hashCounts))
}

func TestHandleInputRequestEvent_OlderUnicityCertificate(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	s.CopyOldInputRecords(string(partition1ID))
	_, err = s.CreateUnicityCertificates()
	resp := SendRequestAndExpectStatus(t, s, req, p1.P1Response_OK)
	require.Greater(t, resp.Message.UnicitySeal.RootChainRoundNumber, req.RootRoundNumber)
}

func TestHandleInputRequestEvent_ExtendingUnknownState(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.InputRecord.PreviousHash = []byte{1, 1, 1}
	err = req.Sign(signers[0])
	require.NoError(t, err)
	resp := SendRequestAndExpectStatus(t, s, req, p1.P1Response_OK)
	require.NotEqual(t, req.InputRecord.PreviousHash, resp.Message.InputRecord.PreviousHash)
}

func TestHandleInputRequestEvent_Duplicated(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	SendRequestAndExpectNeverReturns(t, s, req)
	SendRequestAndExpectStatus(t, s, req, p1.P1Response_DUPLICATE)
}

func TestHandleInputRequestEvent_UnknownSystemIdentifier(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.SystemIdentifier = []byte{0, 0, 0xAA, 0xAA}
	SendRequestAndExpectStatus(t, s, req, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_PartitionHasNewerUC(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.RootRoundNumber = 101
	require.NoError(t, req.Sign(signers[0]))
	SendRequestAndExpectStatus(t, s, req, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_UnknownNodeID(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.NodeIdentifier = "unknown"
	SendRequestAndExpectStatus(t, s, req, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_InputRecordMissing(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	_, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	SendRequestAndExpectStatus(t, s, nil, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_InvalidSignature(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	invalidReq := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	invalidReq.InputRecord.Hash = make([]byte, 32)
	SendRequestAndExpectStatus(t, s, invalidReq, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_DuplicateWithDifferentHash(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 5)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])

	SendRequestAndExpectNeverReturns(t, s, req)
	invalidReq := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	invalidReq.InputRecord.Hash = make([]byte, 32)
	require.NoError(t, invalidReq.Sign(signers[0]))
	SendRequestAndExpectStatus(t, s, invalidReq, p1.P1Response_INVALID)
}

func TestHandleInputRequestEvent_ConsensusNotPossible(t *testing.T) {
	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)
	signers, _, partition1 := createPartitionRecord(t, partition1IR, partition1ID, 3)
	partitions := []*genesis.PartitionRecord{partition1}
	s := createStateAndExecuteRound(t, partitions, rootSigner)
	req := CreateP1Request(partition1.Validators[0].NodeIdentifier, s, signers[0])
	req.InputRecord.Hash = []byte{0, 0, 0}
	err = req.Sign(signers[0])
	require.NoError(t, err)
	req1 := CreateP1Request(partition1.Validators[1].NodeIdentifier, s, signers[0])
	req1.InputRecord.Hash = []byte{1, 1, 1}
	err = req1.Sign(signers[1])
	require.NoError(t, err)
	req2 := CreateP1Request(partition1.Validators[2].NodeIdentifier, s, signers[0])
	req2.InputRecord.Hash = []byte{2, 2, 2}
	err = req2.Sign(signers[2])
	require.NoError(t, err)

	responseCh := make(chan *p1.P1Response, 1)
	s.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req,
		ResponseCh: responseCh,
	})
	responseCh1 := make(chan *p1.P1Response, 1)
	s.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req1,
		ResponseCh: responseCh1,
	})
	responseCh2 := make(chan *p1.P1Response, 1)
	s.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req2,
		ResponseCh: responseCh2,
	})
	_, err = s.CreateUnicityCertificates()
	require.NoError(t, err)
	assert.Eventually(t, func() bool {
		res := <-responseCh
		require.Equal(t, p1.P1Response_OK, res.Status)
		require.NotEqual(t, req.InputRecord.Hash, res.Message.InputRecord.Hash)
		return true
	}, 100*time.Millisecond, 50)
	assert.Eventually(t, func() bool {
		res := <-responseCh1
		require.Equal(t, p1.P1Response_OK, res.Status)
		require.NotEqual(t, req1.InputRecord.Hash, res.Message.InputRecord.Hash)
		return true
	}, 100*time.Millisecond, 50)
	assert.Eventually(t, func() bool {
		res := <-responseCh2
		require.NotEqual(t, req2.InputRecord.Hash, res.Message.InputRecord.Hash)
		return true
	}, 100*time.Millisecond, 50)
}

func createStateAndExecuteRound(t *testing.T, partitions []*genesis.PartitionRecord, rootSigner *crypto.InMemorySecp256K1Signer) *State {
	t.Helper()
	s2, err := NewStateFromPartitionRecords(partitions, rootSigner, gocrypto.SHA256)
	require.NoError(t, err)
	for _, p := range partitions {
		id := string(p.SystemDescriptionRecord.SystemIdentifier)
		s2.checkConsensus(id)
	}
	_, err = s2.CreateUnicityCertificates()
	require.NoError(t, err)
	return s2
}

func createPartitionRecord(t *testing.T, ir *certificates.InputRecord, systemID []byte, nrOfValidators int) (signers []crypto.Signer, verifiers []crypto.Verifier, record *genesis.PartitionRecord) {
	record = &genesis.PartitionRecord{
		SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
			SystemIdentifier: systemID,
			T2Timeout:        2500,
		},
		Validators: []*genesis.PartitionNode{},
	}

	for i := 0; i < nrOfValidators; i++ {
		partition1Signer, err := crypto.NewInMemorySecp256K1Signer()
		require.NoError(t, err)
		verifier, err := partition1Signer.Verifier()
		require.NoError(t, err)
		pubKey, err := verifier.MarshalPublicKey()
		require.NoError(t, err)

		p1Req := &p1.P1Request{
			SystemIdentifier: systemID,
			NodeIdentifier:   fmt.Sprint(i),
			RootRoundNumber:  1,
			InputRecord:      ir,
		}
		err = p1Req.Sign(partition1Signer)
		require.NoError(t, err)

		record.Validators = append(record.Validators, &genesis.PartitionNode{
			NodeIdentifier: fmt.Sprint(i),
			PublicKey:      pubKey,
			P1Request:      p1Req,
		})

		signers = append(signers, partition1Signer)
		verifiers = append(verifiers, verifier)
	}
	return
}

func CreateP1Request(nodeIdentifier string, s *State, signers crypto.Signer) *p1.P1Request {
	req := &p1.P1Request{
		SystemIdentifier: partition1ID,
		NodeIdentifier:   nodeIdentifier,
		RootRoundNumber:  s.roundNumber - 1,
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

func SendRequestAndExpectNeverReturns(t *testing.T, s *State, req *p1.P1Request) {
	responseCh := make(chan *p1.P1Response, 1)
	s.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req,
		ResponseCh: responseCh,
	})

	require.Never(t, func() bool {
		<-responseCh
		return true
	}, 100*time.Millisecond, 50)
}

func SendRequestAndExpectStatus(t *testing.T, s *State, req *p1.P1Request, status p1.P1Response_Status) *p1.P1Response {
	t.Helper()
	responseCh := make(chan *p1.P1Response, 1)
	s.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req,
		ResponseCh: responseCh,
	})
	var resp *p1.P1Response
	require.Eventually(t, func() bool {
		resp = <-responseCh
		require.Equal(t, status, resp.Status)
		return true
	}, 100*time.Millisecond, 50)
	return resp
}
