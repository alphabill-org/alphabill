package monolithic

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testgenesis "github.com/alphabill-org/alphabill/internal/testutils/genesis"
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

func readResult(ch <-chan *certification.CertificationResponse, timeout time.Duration) (*certification.CertificationResponse, error) {
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
	partitionStore, err := partitions.NewPartitionStore(testgenesis.NewGenesisStore(rootGenesis))
	require.NoError(t, err)
	trustBase, err := rootGenesis.GenerateTrustBase()
	require.NoError(t, err)
	cm, err := NewMonolithicConsensusManager(rootNode.PeerConf.ID.String(), trustBase, rootGenesis, partitionStore, rootNode.Signer, testlogger.New(t).With(logger.NodeID(id)), consensus.WithStorage(db))
	require.NoError(t, err)
	return cm, rootNode, partitionNodes, rootGenesis
}

func TestConsensusManager_checkT2Timeout(t *testing.T) {
	partitionStore, err := partitions.NewPartitionStore(
		testgenesis.NewGenesisStoreFromPartitions(
			[]*genesis.GenesisPartitionRecord{
				{PartitionDescription: &types.PartitionDescriptionRecord{NetworkIdentifier: 5, SystemIdentifier: sysID3, T2Timeout: 2500 * time.Millisecond}},
				{PartitionDescription: &types.PartitionDescriptionRecord{NetworkIdentifier: 5, SystemIdentifier: sysID1, T2Timeout: 2500 * time.Millisecond}},
				{PartitionDescription: &types.PartitionDescriptionRecord{NetworkIdentifier: 5, SystemIdentifier: sysID2, T2Timeout: 2500 * time.Millisecond}},
			}))
	require.NoError(t, err)
	require.NoError(t, partitionStore.Reset(func() uint64 { return 1 }))
	db, err := memorydb.New()
	require.NoError(t, err)
	store := NewStateStore(db)
	// store mock state
	certs := []*certification.CertificationResponse{
		{
			Partition: sysID3,
			UC: types.UnicityCertificate{
				InputRecord:            &types.InputRecord{Hash: []byte{1, 1}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
				UnicityTreeCertificate: &types.UnicityTreeCertificate{},
				UnicitySeal:            &types.UnicitySeal{Version: 1, RootChainRoundNumber: 3, Hash: []byte{1}}}, // no timeout (5 - 3) * 900 = 1800 ms,
		},
		{
			Partition: sysID1,
			UC: types.UnicityCertificate{
				InputRecord:            &types.InputRecord{Hash: []byte{1, 2}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
				UnicityTreeCertificate: &types.UnicityTreeCertificate{},
				UnicitySeal:            &types.UnicitySeal{Version: 1, RootChainRoundNumber: 2, Hash: []byte{1}}}, // timeout (5 - 2) * 900 = 2700 ms
		},
		{
			Partition: sysID2,
			UC: types.UnicityCertificate{
				InputRecord:            &types.InputRecord{Hash: []byte{1, 3}, PreviousHash: []byte{1, 1}, BlockHash: []byte{2, 3}, SummaryValue: []byte{3, 4}},
				UnicityTreeCertificate: &types.UnicityTreeCertificate{},
				UnicitySeal:            &types.UnicitySeal{Version: 1, RootChainRoundNumber: 4, Hash: []byte{1}}}, // no timeout
		},
	}
	// shortcut, to avoid generating root genesis file
	require.NoError(t, store.save(4, certs))

	manager := &ConsensusManager{
		selfID: "test",
		params: &consensus.Parameters{
			BlockRate: 900 * time.Millisecond, // also known as T3
		},
		partitions: partitionStore,
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
	cm, _, partitionNodes, rg := initConsensusManager(t, db)
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
		Partition: partitionID,
		Reason:    consensus.Quorum,
		Requests:  requests}
	// submit IR change request from partition with quorum
	require.NoError(t, cm.RequestCertification(ctx, req))
	// require, that certificates are received for partition ID
	result, err := readResult(cm.CertificationResult(), 2000*time.Millisecond)
	require.NoError(t, err)
	require.Equal(t, partitionInputRecord.Hash, result.UC.InputRecord.PreviousHash)
	require.Equal(t, uint64(2), result.UC.UnicitySeal.RootChainRoundNumber)
	require.NotNil(t, result.UC.UnicitySeal.Hash)
	trustBase, err := rg.GenerateTrustBase()
	require.NoError(t, err)
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.UC.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
	cert, err := cm.GetLatestUnicityCertificate(partitionID, types.ShardID{})
	require.NoError(t, err)
	require.Equal(t, cert, result)

	// ask for non-existing cert
	cert, err = cm.GetLatestUnicityCertificate(types.SystemID(10), types.ShardID{})
	require.EqualError(t, err, "loading certificate for partition 0000000A from state store: no certificate for partition 0000000A in DB")
	require.Nil(t, cert)
}

func TestConsensusManager_PersistFails(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	cm, _, partitionNodes, rg := initConsensusManager(t, db)
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
		Partition: partitionID,
		Reason:    consensus.Quorum,
		Requests:  requests}
	// set db to simulate error
	db.MockWriteError(fmt.Errorf("db write failed"))
	// submit IR change request from partition with quorum
	require.NoError(t, cm.onIRChangeReq(req))
	require.Equal(t, uint64(1), cm.round)
	cert, err := cm.GetLatestUnicityCertificate(partitionID, types.ShardID{})
	require.NoError(t, err)
	// simulate T3 timeout
	cm.onT3Timeout(context.Background())
	// due to DB persist error round is not incremented and will be repeated
	require.Equal(t, uint64(1), cm.round)
	certNow, err := cm.GetLatestUnicityCertificate(partitionID, types.ShardID{})
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
	require.Equal(t, partitionInputRecord.Hash, result.UC.InputRecord.PreviousHash)
	require.Equal(t, uint64(2), result.UC.UnicitySeal.RootChainRoundNumber)
	require.NotNil(t, result.UC.UnicitySeal.Hash)
	trustBase, err := rg.GenerateTrustBase()
	require.NoError(t, err)
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.UC.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
}

// this will run long, cut timeouts or find a way to manipulate timeouts
func TestConsensusManager_PartitionTimeout(t *testing.T) {
	dir := t.TempDir()
	boltDb, err := boltdb.New(filepath.Join(dir, "bolt.db"))
	require.NoError(t, err)
	cm, _, partitionNodes, rg := initConsensusManager(t, boltDb)
	ctx, ctxCancel := context.WithCancel(context.Background())
	defer ctxCancel()
	go func() { require.ErrorIs(t, cm.Run(ctx), context.Canceled) }()

	// make sure that 3 partition nodes where generated, needed for the next steps
	require.Len(t, partitionNodes, 3)
	cert, err := cm.GetLatestUnicityCertificate(partitionID, types.ShardID{})
	require.NoError(t, err)
	require.NotNil(t, cert)
	require.Equal(t, cert.UC.InputRecord.Bytes(), partitionInputRecord.Bytes())
	// require, that repeat UC certificates are received for partition ID in 3 root rounds (partition timeout 2500 < 4 * block rate (900))
	result, err := readResult(cm.CertificationResult(), 4*cm.params.BlockRate)
	require.NoError(t, err)
	require.Equal(t, cert.UC.InputRecord.Bytes(), result.UC.InputRecord.Bytes())
	require.NotNil(t, result.UC.UnicitySeal.Hash)
	// ensure repeat UC is created in a different rc round
	require.GreaterOrEqual(t, result.UC.UnicitySeal.RootChainRoundNumber, cert.UC.UnicitySeal.RootChainRoundNumber)
	trustBase, err := rg.GenerateTrustBase()
	require.NoError(t, err)
	sdrh := rg.Partitions[0].GetSystemDescriptionRecord().Hash(gocrypto.SHA256)
	require.NoError(t, result.UC.Verify(trustBase, gocrypto.SHA256, partitionID, sdrh))
}
