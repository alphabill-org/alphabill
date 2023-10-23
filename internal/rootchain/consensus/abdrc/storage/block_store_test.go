package storage

import (
	gocrypto "crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

type MockAlwaysOkBlockVerifier struct {
	blockStore *BlockStore
}

func NewAlwaysOkBlockVerifier(bStore *BlockStore) *MockAlwaysOkBlockVerifier {
	return &MockAlwaysOkBlockVerifier{
		blockStore: bStore,
	}
}

func (m *MockAlwaysOkBlockVerifier) VerifyIRChangeReq(_ uint64, irChReq *abtypes.IRChangeReq) (*InputData, error) {
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest
	// Remember all partitions that have changes in the current proposal and apply changes
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	luc, err := m.blockStore.GetCertificate(irChReq.SystemIdentifier)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: partition %s state is missing: %w", irChReq.SystemIdentifier, err)
	}
	switch irChReq.CertReason {
	case abtypes.Quorum:
		// NB! there was at least one request, otherwise we would not be here
		return &InputData{IR: irChReq.Requests[0].InputRecord, Sdrh: luc.UnicityTreeCertificate.SystemDescriptionHash}, nil
	case abtypes.QuorumNotPossible:
	case abtypes.T2Timeout:
		return &InputData{SysID: irChReq.SystemIdentifier, IR: luc.InputRecord, Sdrh: luc.UnicityTreeCertificate.SystemDescriptionHash}, nil
	}
	return nil, fmt.Errorf("unknown certification reason %v", irChReq.CertReason)
}

func initBlockStoreFromGenesis(t *testing.T) *BlockStore {
	t.Helper()
	bStore, err := NewBlockStore(gocrypto.SHA256, pg, memorydb.New())
	require.NoError(t, err)
	return bStore
}

func TestNewBlockStoreFromGenesis(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(1), hQc.VoteInfo.RoundNumber)
	require.False(t, bStore.IsChangeInProgress(sysID1))
	rh, err := bStore.GetBlockRootHash(1)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	_, err = bStore.GetBlockRootHash(2)
	require.ErrorContains(t, err, "block for round 2 not found")
	require.Len(t, bStore.GetCertificates(), 2)
}

func TestHandleTcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
	require.ErrorContains(t, bStore.ProcessTc(nil), "tc is nil")
	tc := &abtypes.TimeoutCert{
		Timeout: &abtypes.Timeout{
			Round: 3,
		},
	}
	// For now ignore if we do not have a block that timeouts
	require.NoError(t, bStore.ProcessTc(tc))
}

func TestHandleQcError(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
	ucs, err := bStore.ProcessQc(nil)
	require.ErrorContains(t, err, "qc is nil")
	require.Nil(t, ucs)
	// qc for non-existing round
	qc := &abtypes.QuorumCert{
		VoteInfo: &abtypes.RoundInfo{
			RoundNumber:       3,
			ParentRoundNumber: 2,
		},
	}
	ucs, err = bStore.ProcessQc(qc)
	require.ErrorContains(t, err, "block for round 3 not found")
	require.Nil(t, ucs)
}

func TestBlockStoreAdd(t *testing.T) {
	bStore := initBlockStoreFromGenesis(t)
	rBlock := bStore.GetRoot()
	mockBlockVer := NewAlwaysOkBlockVerifier(bStore)
	block := &abtypes.BlockData{
		Round:   genesis.RootRound,
		Payload: &abtypes.Payload{},
		Qc:      nil,
	}
	_, err := bStore.Add(block, mockBlockVer)
	require.ErrorContains(t, err, "add block failed: different block for round 1 is already in store")
	block = &abtypes.BlockData{
		Round:   genesis.RootRound + 1,
		Payload: &abtypes.Payload{},
		Qc:      bStore.GetHighQc(), // the qc is genesis qc
	}
	// Proposal always comes with Qc and Block, process Qc first and then new block
	// add qc for block 1
	ucs, err := bStore.ProcessQc(block.Qc)
	require.NoError(t, err)
	require.Nil(t, ucs)
	// and the new block 2
	rh, err := bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	// add qc for round 3, commits round 1
	vInfo := &abtypes.RoundInfo{
		RoundNumber:       genesis.RootRound + 1,
		ParentRoundNumber: genesis.RootRound,
		CurrentRootHash:   rh,
	}
	qc := &abtypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Hash: rBlock.RootHash,
		},
	}
	// qc for round 2, does not commit a round
	block = &abtypes.BlockData{
		Round:   genesis.RootRound + 2,
		Payload: &abtypes.Payload{},
		Qc:      qc,
	}
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)
	// add block 3
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	// prepare block 4
	vInfo = &abtypes.RoundInfo{
		RoundNumber:       genesis.RootRound + 2,
		ParentRoundNumber: genesis.RootRound + 1,
		CurrentRootHash:   rh,
	}
	qc = &abtypes.QuorumCert{
		VoteInfo: vInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Hash: rBlock.RootHash,
		},
	}
	// qc for round 2, does not commit a round
	block = &abtypes.BlockData{
		Round:   genesis.RootRound + 3,
		Payload: &abtypes.Payload{},
		Qc:      qc,
	}
	// Add
	ucs, err = bStore.ProcessQc(qc)
	require.NoError(t, err)
	require.Empty(t, ucs)
	// add block 4
	rh, err = bStore.Add(block, mockBlockVer)
	require.NoError(t, err)
	require.Len(t, rh, 32)
	rBlock = bStore.GetRoot()
	// block in round 2 becomes root
	require.Equal(t, uint64(2), rBlock.BlockData.Round)
	hQc := bStore.GetHighQc()
	require.Equal(t, uint64(3), hQc.VoteInfo.RoundNumber)
}
