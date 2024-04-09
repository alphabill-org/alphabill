package monolithic

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/keyvaluedb/boltdb"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

const sysID1 types.SystemID = 1
const sysID2 types.SystemID = 2
const sysID3 types.SystemID = 5
const partitionID types.SystemID = 0x00FF0001

var partitionInputRecord = &types.InputRecord{
	PreviousHash: make([]byte, 32),
	Hash:         []byte{0, 0, 0, 1},
	BlockHash:    []byte{0, 0, 1, 2},
	SummaryValue: []byte{0, 0, 1, 3},
	RoundNumber:  1,
}

func readResult(ch <-chan *types.UnicityCertificate, timeout time.Duration) (*types.UnicityCertificate, error) {
	select {
	case result, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("failed to read from channel")
		}
		return result, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("timeout")
	}
}

func initConsensusManager(t *testing.T, db keyvaluedb.KeyValueDB) (*ConsensusManager, *testutils.TestNode, []*testutils.TestNode, *genesis.RootGenesis) {
	partitionNodes, partitionRecord := testutils.CreatePartitionNodesAndPartitionRecord(t, partitionInputRecord, partitionID, 3)
	rootNode := testutils.NewTestNode(t)
	verifier := rootNode.Verifier
	rootPubKeyBytes, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	id := rootNode.PeerConf.ID
	rootGenesis, _, err := rootgenesis.NewRootGenesis(id.String(), rootNode.Signer, rootPubKeyBytes, []*genesis.PartitionRecord{partitionRecord})
	require.NoError(t, err)
	partitions, err := partitions.NewPartitionStoreFromGenesis(rootGenesis.Partitions)
	require.NoError(t, err)
	cm, err := NewMonolithicConsensusManager(rootNode.PeerConf.ID.String(), rootGenesis, partitions, rootNode.Signer, testlogger.New(t).With(logger.NodeID(id)), consensus.WithStorage(db))
	require.NoError(t, err)
	return cm, rootNode, partitionNodes, rootGenesis
}

func TestConsensusManager_checkT2Timeout(t *testing.T) {
	partitions, err := partitions.NewPartitionStoreFromGenesis([]*genesis.GenesisPartitionRecord{
		{SystemDescriptionRecord: &types.SystemDescriptionRecord{SystemIdentifier: sysID3, T2Timeout: 2500}},
		{SystemDescriptionRecord: &types.SystemDescriptionRecord{SystemIdentifier: sysID1, T2Timeout: 2500}},
		{SystemDescriptionRecord: &types.SystemDescriptionRecord{SystemIdentifier: sysID2, T2Timeout: 2500}},
	})
	require.NoError(t, err)
	db, err := memorydb.New()
	require.NoError(t, err)
	store := NewStateStore(db)
	// store mock state
	certs := map[types.SystemID]*types.UnicityCertificate{
		sysID3: {
			InputRecord:            &types.InputRecord{Hash: []byte{1, 1}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{},
			UnicitySeal:            &types.UnicitySeal{RootChainRoundNumber: 3}}, // no timeout (5 - 3) * 900 = 1800 ms
		sysID1: {
			InputRecord:            &types.InputRecord{Hash: []byte{1, 2}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{},
			UnicitySeal:            &types.UnicitySeal{RootChainRoundNumber: 2}}, // timeout (5 - 2) * 900 = 2700 ms
		sysID2: {
			InputRecord:            &types.InputRecord{Hash: []byte{1, 3}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{},
			UnicitySeal:            &types.UnicitySeal{RootChainRoundNumber: 4}}, // no timeout
	}
	// shortcut, to avoid generating root genesis file
	require.NoError(t, store.save(4, certs))

	manager := &ConsensusManager{
		selfID: "test",
		params: &consensus.Parameters{
			BlockRate: 900 * time.Millisecond, // also known as T3
		},
		partitions: partitions,
		stateStore: store,
		ir: map[types.SystemID]*types.InputRecord{
			sysID3: {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			sysID1: {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
			sysID2: {Hash: []byte{0, 1}, PreviousHash: []byte{0, 0}, BlockHash: []byte{1, 2}, SummaryValue: []byte{2, 3}},
		},
		changes: map[types.SystemID]*types.InputRecord{},
		log:     testlogger.New(t),
	}
	require.NoError(t, manager.checkT2Timeout(5))
	// if round is 900ms then timeout of 2500 is reached in 3 * 900ms rounds, which is 2700ms
	require.Equal(t, 1, len(manager.changes))
	require.NoError(t, err)
	require.Contains(t, manager.changes, sysID1)
}

func TestConsensusManager_NormalOperation(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, db)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	// mock requests from partition node
	requests := make([]*certification.BlockCertificationRequest, 2)

	ir := rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord
	newIR := &types.InputRecord{
		PreviousHash: ir.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: ir.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	req := consensus.IRChangeRequest{
		SystemIdentifier: partitionID,
		Reason:           consensus.Quorum,
		Requests:         requests}
	// submit IR change request from partition with quorum
	require.NoError(t, cm.RequestCertification(ctx, req))
	// require, that certificates are received for partition ID
	result, err := readResult(cm.CertificationResult(), 2000*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, partitionInputRecord.Hash, result.InputRecord.PreviousHash)
	require.Equal(t, uint64(2), result.UnicitySeal.RootChainRoundNumber)
	require.NotNil(t, result.UnicitySeal.Hash)
	trustBase := map[string]crypto.Verifier{rootNode.PeerConf.ID.String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
	cert, err := cm.GetLatestUnicityCertificate(partitionID)
	require.NoError(t, err)
	require.Equal(t, cert, result)

	// ask for non-existing cert
	cert, err = cm.GetLatestUnicityCertificate(types.SystemID(10))
	require.EqualError(t, err, "loading certificate for partition 0000000A from state store: no certificate for partition 0000000A in DB")
	require.Nil(t, cert)
}

func TestConsensusManager_PersistFails(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, db)
	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	// mock requests from partition node
	requests := make([]*certification.BlockCertificationRequest, 2)
	ir := rg.Partitions[0].Nodes[0].BlockCertificationRequest.InputRecord
	newIR := &types.InputRecord{
		PreviousHash: ir.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: ir.SummaryValue,
		RoundNumber:  1,
	}
	requests[0] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[0])
	requests[1] = testutils.CreateBlockCertificationRequest(t, newIR, partitionID, partitionNodes[1])
	req := &consensus.IRChangeRequest{
		SystemIdentifier: partitionID,
		Reason:           consensus.Quorum,
		Requests:         requests}
	// set db to simulate error
	db.MockWriteError(fmt.Errorf("db write failed"))
	// submit IR change request from partition with quorum
	require.NoError(t, cm.onIRChangeReq(req))
	require.Equal(t, uint64(1), cm.round)
	cert, err := cm.GetLatestUnicityCertificate(partitionID)
	require.NoError(t, err)
	// simulate T3 timeout
	cm.onT3Timeout(context.Background())
	// due to DB persist error round is not incremented and will be repeated
	require.Equal(t, uint64(1), cm.round)
	certNow, err := cm.GetLatestUnicityCertificate(partitionID)
	require.NoError(t, err)

	require.Equal(t, cert, certNow)
	// simulate T3 timeout
	cm.onT3Timeout(context.Background())
	// same deal no progress
	require.Equal(t, uint64(1), cm.round)
	// set db to simulate error
	db.MockWriteError(nil)
	// start run and make sure rootchain recovers as it is now able to persist
	// new state and hence a certificate should be now received
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()
	// require, that certificates are received for partition ID
	result, err := readResult(cm.CertificationResult(), 2*cm.params.BlockRate)
	require.NoError(t, err)
	require.Equal(t, partitionInputRecord.Hash, result.InputRecord.PreviousHash)
	require.Equal(t, uint64(2), result.UnicitySeal.RootChainRoundNumber)
	require.NotNil(t, result.UnicitySeal.Hash)
	trustBase := map[string]crypto.Verifier{rootNode.PeerConf.ID.String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
}

// this will run long, cut timeouts or find a way to manipulate timeouts
func TestConsensusManager_PartitionTimeout(t *testing.T) {
	dir := t.TempDir()
	boltDb, err := boltdb.New(filepath.Join(dir, "bolt.db"))
	require.NoError(t, err)
	cm, rootNode, partitionNodes, rg := initConsensusManager(t, boltDb)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	cert, err := cm.GetLatestUnicityCertificate(partitionID)
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, cert.InputRecord.Bytes(), partitionInputRecord.Bytes())
	// require, that repeat UC certificates are received for partition ID in 3 root rounds (partition timeout 2500 < 4 * block rate (900))
	result, err := readResult(cm.CertificationResult(), 4*cm.params.BlockRate)
	require.NoError(t, err)
	require.Equal(t, cert.InputRecord.Bytes(), result.InputRecord.Bytes())
	require.NotNil(t, result.UnicitySeal.Hash)
	// ensure repeat UC is created in a different rc round
	require.GreaterOrEqual(t, result.UnicitySeal.RootChainRoundNumber, cert.UnicitySeal.RootChainRoundNumber)
	trustBase := map[string]crypto.Verifier{rootNode.PeerConf.ID.String(): rootNode.Verifier}
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
}
