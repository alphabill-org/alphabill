package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"crypto/rand"
	"fmt"
	"strings"
	"testing"
	"time"

	p2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testlogger "github.com/alphabill-org/alphabill/internal/testutils/logger"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/txsystem"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

type SingleNodePartition struct {
	nodeConf   *NodeConf
	txSystem   txsystem.TransactionSystem
	node       *Node

	rootNodeID string
	rootRound  uint64
	rootSigner crypto.Signer
	certs      map[types.PartitionID]*types.UnicityCertificate

	mockNet    *testnetwork.MockNet
	eh         *testevent.TestEventHandler
	obs        Observability
}

type partitionStartupDependencies struct {
	nodeOptions []NodeOption
}

func (t *AlwaysValidTransactionValidator) Validate(_ *types.TransactionOrder, _ uint64) error {
	return nil
}

func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

func newSingleValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return newSingleNodeShard(t, txSystem, true, nodeOptions...)
}

func newSingleNonValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return newSingleNodeShard(t, txSystem, false, nodeOptions...)
}

func newSingleNodeShard(t *testing.T, txSystem txsystem.TransactionSystem, validator bool, nodeOptions ...NodeOption) *SingleNodePartition {
	// the only running node
	keyConf, nodeInfo := createKeyConf(t)
	nodeID, err := keyConf.NodeID()
	require.NoError(t, err)

	// the fake validator, so that network 'send' requests don't fail
	_, fakeNodeInfo := createKeyConf(t)

	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     0x01010101,
		ShardID:         types.ShardID{},
		PartitionTypeID: 999,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		Epoch:           0,
		EpochStart:      1,
		Validators:      []*types.NodeInfo{fakeNodeInfo},
	}

	if validator {
		shardConf.Validators = append(shardConf.Validators, nodeInfo)
	}

	// trust base
	rootSigner, rootVerifier := testsig.CreateSignerAndVerifier(t)
	trustBase := trustbase.NewTrustBase(t, rootVerifier)
	net := testnetwork.NewMockNetwork(t)
	log := testlogger.New(t).With(logger.NodeID(nodeID))
	obs := observability.WithLogger(testobserve.Default(t), log)
	eh := &testevent.TestEventHandler{}

	nodeOptions = append([]NodeOption{
		WithT1Timeout(100 * time.Minute),
		WithTxValidator(&AlwaysValidTransactionValidator{}),
		WithEventHandler(eh.HandleEvent, 100),
		WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		WithValidatorNetwork(net),
	}, nodeOptions...)

	nodeConf, err := NewNodeConf(keyConf, shardConf, trustBase, obs, nodeOptions...)
	require.NoError(t, err)

	shard := &SingleNodePartition{
		nodeConf:   nodeConf,
		txSystem:   txSystem,
		rootRound:  1,
		certs:      make(map[types.PartitionID]*types.UnicityCertificate),
		rootSigner: rootSigner,
		rootNodeID: trustBase.GetRootNodes()[0].NodeID,
		mockNet:    net,
		eh:         eh,
		obs:        obs,
	}

	shard.createInitialUC(t)
	return shard
}

func runSingleValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return runSingleNodePartition(t, txSystem, true, nodeOptions...)
}

func runSingleNonValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return runSingleNodePartition(t, txSystem, false, nodeOptions...)
}

func runSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, validator bool, nodeOptions ...NodeOption) *SingleNodePartition {
	ctx, cancel := context.WithCancel(context.Background())

	shard := newSingleNodeShard(t, txSystem, validator, nodeOptions...)
	done := shard.start(ctx, t)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("shard node didn't shut down within timeout")
		}
	})
	return shard
}

func (s *SingleNodePartition) createInitialUC(t *testing.T) {
	initialUC, _, err := s.CreateUnicityCertificate(t, &types.InputRecord{Version: 1}, 1)
	require.NoError(t, err)
	s.certs[s.nodeConf.shardConf.PartitionID] = initialUC
	require.NoError(t, s.txSystem.Commit(initialUC))
}

func (sn *SingleNodePartition) start(ctx context.Context, t *testing.T) chan struct{} {
	done := make(chan struct{})
	node, err := NewNode(context.Background(), sn.txSystem, sn.nodeConf)
	require.NoError(t, err, "failed to init shard node")
	sn.node = node

	go func() {
		require.ErrorIs(t, sn.node.Run(ctx), context.Canceled)
		close(done)
	}()
	return done
}
func (sn *SingleNodePartition) nodeID(t *testing.T) peer.ID {
	nodeID, err := sn.nodeConf.keyConf.NodeID()
	require.NoError(t, err)
	return nodeID
}

func (sn *SingleNodePartition) SubmitTx(tx *types.TransactionOrder) error {
	sn.mockNet.AddTransaction(context.Background(), tx)
	return nil
}

func (sn *SingleNodePartition) SubmitTxFromRPC(tx *types.TransactionOrder) error {
	_, err := sn.node.SubmitTx(context.Background(), tx)
	return err
}

func (sn *SingleNodePartition) ReceiveCertResponseSameEpoch(t *testing.T, ir *types.InputRecord, rootRoundNumber uint64) {
	sn.ReceiveCertResponse(t, ir, rootRoundNumber, ir.Epoch)
}

func (sn *SingleNodePartition) ReceiveCertResponseWithEpoch(t *testing.T, ir *types.InputRecord, rootRoundNumber uint64, epoch uint64) {
	sn.ReceiveCertResponse(t, ir, rootRoundNumber, epoch)
}

/*
ReceiveCertResponse builds UC and TR based on given input and "sends" them as CertificationResponse to the node.
*/
func (sn *SingleNodePartition) ReceiveCertResponse(t *testing.T, ir *types.InputRecord, rootRoundNumber uint64, epoch uint64) {
	uc, tr, err := sn.CreateUnicityCertificateTR(t, ir, rootRoundNumber, epoch)
	if err != nil {
		t.Fatalf("creating UC and TR: %v", err)
	}

	sn.mockNet.Receive(&certification.CertificationResponse{
		Partition: sn.nodeConf.GetPartitionID(),
		Shard:     sn.nodeConf.shardConf.ShardID,
		Technical: tr,
		UC:        *uc,
	})
}

/*
SubmitUnicityCertificate wraps the UC into CertificationResponse and sends it to the node.
*/
func (sn *SingleNodePartition) SubmitUnicityCertificate(t *testing.T, uc *types.UnicityCertificate) {
	cr := &certification.CertificationResponse{
		Partition: sn.nodeConf.GetPartitionID(),
		Shard:     sn.nodeConf.shardConf.ShardID,
		UC:        *uc,
	}
	tr := technicalRecord(t, uc.InputRecord, []string{sn.node.peer.ID().String()})
	if err := cr.SetTechnicalRecord(tr); err != nil {
		t.Fatalf("setting TR of the CertResp: %v", err)
	}

	sn.mockNet.Receive(cr)
}

/*
WaitHandshake waits until partition node sends handshake message to the RootChain
and responds to it with the genesis UC. After that validator should be ready for
normal operation.
*/
func (sn *SingleNodePartition) WaitHandshake(t *testing.T) {
	test.TryTilCountIs(t, RequestReceived(sn, network.ProtocolHandshake), 5, test.WaitShortTick)
	sn.mockNet.ResetSentMessages(network.ProtocolHandshake)

	cr := &certification.CertificationResponse{
		Partition: sn.nodeConf.shardConf.PartitionID,
		Shard:     sn.nodeConf.shardConf.ShardID,
	}
	uc := sn.certs[sn.node.PartitionID()]
	// TODO: makes it a duplicate, same uc already in txs as commitedUC, is this needed?
	cr.UC = *uc
	cr.Technical = technicalRecord(t, uc.InputRecord, []string{sn.nodeID(t).String()})

	if err := cr.IsValid(); err != nil {
		t.Errorf("invalid CertRsp: %v", err)
	}
	if err := sn.node.handleMessage(context.Background(), cr); err != nil {
		t.Errorf("sending handshake response to the node: %v", err)
	}
}

func (sn *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) {
	sn.mockNet.Receive(prop)
}

func (sn *SingleNodePartition) CreateUnicityCertificate(t *testing.T, ir *types.InputRecord, rootRound uint64) (*types.UnicityCertificate, *certification.TechnicalRecord, error) {
	tr := technicalRecord(t, ir, []string{sn.nodeID(t).String()})
	trHash, err := tr.Hash()
	if err != nil {
		return nil, nil, fmt.Errorf("calculating TechnicalRecord hash: %w", err)
	}

	shardConf := sn.nodeConf.shardConf
	shardConfHash, err := shardConf.Hash(gocrypto.SHA256)
	if err != nil {
		return nil, nil, fmt.Errorf("calculating PDR hash: %w", err)
	}
	sTree, err := types.CreateShardTree(nil, []types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
	if err != nil {
		return nil, nil, fmt.Errorf("creating shard tree: %w", err)
	}
	stCert, err := sTree.Certificate(types.ShardID{})
	if err != nil {
		return nil, nil, fmt.Errorf("creating shard tree certificate: %w", err)
	}

	data := []*types.UnicityTreeData{{
		Partition:     shardConf.PartitionID,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       shardConfHash,
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		return nil, nil, err
	}
	rootHash := ut.RootHash()
	unicitySeal, err := sn.createUnicitySeal(rootRound, rootHash)
	if err != nil {
		return nil, nil, err
	}
	cert, err := ut.Certificate(shardConf.PartitionID)
	if err != nil {
		return nil, nil, fmt.Errorf("creating UnicityTreeCertificate: %w", err)
	}

	return &types.UnicityCertificate{
		Version:                1,
		InputRecord:            ir,
		TRHash:                 trHash,
		ShardTreeCertificate:   stCert,
		UnicityTreeCertificate: cert,
		UnicitySeal:            unicitySeal,
	}, &tr, nil
}

func (sn *SingleNodePartition) CreateUnicityCertificateTR(t *testing.T, ir *types.InputRecord, rootRoundNumber uint64, epoch uint64) (*types.UnicityCertificate, certification.TechnicalRecord, error) {
	tr := technicalRecord(t, ir, []string{sn.nodeID(t).String()})
	tr.Epoch = epoch
	trHash, err := tr.Hash()
	if err != nil {
		return nil, tr, fmt.Errorf("calculating TechnicalRecord hash: %w", err)
	}

	shardConf := sn.nodeConf.shardConf
	shardConfHash, err := shardConf.Hash(gocrypto.SHA256)
	if err != nil {
		return nil, tr, fmt.Errorf("calculating PDR hash: %w", err)
	}
	sTree, err := types.CreateShardTree(
		nil,
		[]types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
	if err != nil {
		return nil, tr, fmt.Errorf("creating shard tree: %w", err)
	}
	stCert, err := sTree.Certificate(types.ShardID{})
	if err != nil {
		return nil, tr, fmt.Errorf("creating shard tree certificate: %w", err)
	}

	data := []*types.UnicityTreeData{{
		Partition:     shardConf.PartitionID,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       shardConfHash,
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		return nil, tr, err
	}
	rootHash := ut.RootHash()
	unicitySeal, err := sn.createUnicitySeal(rootRoundNumber, rootHash)
	if err != nil {
		return nil, tr, err
	}
	cert, err := ut.Certificate(shardConf.PartitionID)
	if err != nil {
		return nil, tr, fmt.Errorf("creating UnicityTreeCertificate: %w", err)
	}

	return &types.UnicityCertificate{
		Version:                1,
		InputRecord:            ir,
		TRHash:                 trHash,
		ShardTreeCertificate:   stCert,
		UnicityTreeCertificate: cert,
		UnicitySeal:            unicitySeal,
	}, tr, nil
}

func (sn *SingleNodePartition) createUnicitySeal(roundNumber uint64, rootHash []byte) (*types.UnicitySeal, error) {
	u := &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: roundNumber,
		Timestamp:            types.NewTimestamp(),
		Hash:                 rootHash,
	}
	return u, u.Sign(sn.rootNodeID, sn.rootSigner)
}

func (sn *SingleNodePartition) GetCommittedUC(t *testing.T) *types.UnicityCertificate {
	uc := sn.node.committedUC()
	require.NotNil(t, uc)
	return uc
}

func (sn *SingleNodePartition) GetLatestBlock(t *testing.T) *types.Block {
	dbIt := sn.node.blockStore.Last()
	defer func() {
		if err := dbIt.Close(); err != nil {
			t.Errorf("Unexpected DB iterator error: %v", err)
		}
	}()
	var bl types.Block
	require.NoError(t, dbIt.Value(&bl))
	return &bl
}

func (sn *SingleNodePartition) CreateBlock(t *testing.T) {
	sn.SubmitT1Timeout(t)
	sn.eh.Reset()
	sn.SubmitUnicityCertificate(t, sn.IssueBlockUC(t))
	testevent.ContainsEvent(t, sn.eh, event.BlockFinalized)
}

func (sn *SingleNodePartition) IssueBlockUC(t *testing.T) *types.UnicityCertificate {
	req := sn.mockNet.SentMessages(network.ProtocolBlockCertification)[0].Message.(*certification.BlockCertificationRequest)
	sn.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	ver, err := sn.nodeConf.signer.Verifier()
	require.NoError(t, err)
	require.NoError(t, req.IsValid(ver))
	rootRound := sn.node.luc.Load().GetRootRoundNumber()
	uc, _, err := sn.CreateUnicityCertificate(t, req.InputRecord, rootRound+1)
	require.NoError(t, err)
	// update state
	sn.rootRound = uc.UnicitySeal.RootChainRoundNumber
	sn.certs[req.PartitionID] = uc
	return uc
}

func (sn *SingleNodePartition) SubmitT1Timeout(t *testing.T) {
	sn.eh.Reset()
	sn.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	sn.node.handleT1TimeoutEvent(context.Background())
	require.Eventually(t, func() bool {
		return len(sn.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
}

func (sn *SingleNodePartition) SubmitMonitorTimeout(t *testing.T) {
	t.Helper()
	sn.eh.Reset()
	sn.node.handleMonitoring(context.Background(), time.Now().Add(-3*sn.nodeConf.GetT2Timeout()), time.Now())
}

func createKeyConf(t *testing.T) (*KeyConf, *types.NodeInfo) {
	privKey, _, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)

	nodeID, err := peer.IDFromPrivateKey(privKey)
	require.NoError(t, err)

	privKeyBytes, err := privKey.Raw()
	require.NoError(t, err)

	key := Key{
		Algorithm: KeyAlgorithmSecp256k1,
		PrivateKey: privKeyBytes,
	}
	keyConf := &KeyConf{
		SigKey: key,
		AuthKey: key,
	}
	signer, err := keyConf.Signer()
	require.NoError(t, err)
	sigVerifier, err := signer.Verifier()
	require.NoError(t, err)
	sigKey, err := sigVerifier.MarshalPublicKey()
	require.NoError(t, err)

	return keyConf, &types.NodeInfo{
		NodeID: nodeID.String(),
		SigKey: sigKey,
		Stake:  1,
	}
}

func NextBlockReceived(t *testing.T, tp *SingleNodePartition, committedUC *types.UnicityCertificate) func() bool {
	t.Helper()
	return func() bool {
		// Empty blocks are not persisted, assume new block is received if new last UC round is bigger than block UC round
		// NB! it could also be that repeat UC is received
		return tp.node.luc.Load().GetRoundNumber() > committedUC.GetRoundNumber()
	}
}

func ContainsTransaction(t *testing.T, block *types.Block, tx *types.TransactionOrder) bool {
	for _, tr := range block.Transactions {
		txBytes, err := tx.MarshalCBOR()
		require.NoError(t, err)
		if bytes.Equal(tr.TransactionOrder, txBytes) {
			return true
		}
	}
	return false
}

func RequestReceived(tp *SingleNodePartition, req string) func() bool {
	return func() bool {
		messages := tp.mockNet.SentMessages(req)
		return len(messages) > 0
	}
}

func ContainsError(t *testing.T, tp *SingleNodePartition, errStr string) {
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.Error && strings.Contains(e.Content.(error).Error(), errStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func ContainsEventType(t *testing.T, tp *SingleNodePartition, evType event.Type) {
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == evType {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

// WaitNodeRequestReceived waits for req type message from node and if there is more than one, copy of the latest is
// returned and the buffer is cleared. NB! if there is already such a message received the method will return with the latest
// immediately. Make sure to clear the "sent" messages if test expects a new message.
func WaitNodeRequestReceived(t *testing.T, tp *SingleNodePartition, req string) *testnetwork.PeerMessage {
	t.Helper()
	defer tp.mockNet.ResetSentMessages(req)
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(req)
		return len(reqs) > 0
	}, test.WaitDuration, test.WaitTick)
	// if more than one return last, but there has to be at least one, otherwise require.Eventually fails before
	return &testnetwork.PeerMessage{
		ID:      reqs[len(reqs)-1].ID,
		Message: reqs[len(reqs)-1].Message,
	}
}

func technicalRecord(t *testing.T, ir *types.InputRecord, nodes []string) certification.TechnicalRecord {
	require.NotEmpty(t, nodes)

	tr := certification.TechnicalRecord{
		Round:  ir.RoundNumber + 1,
		Epoch:  ir.Epoch,
		Leader: nodes[0],
		// precalculated hash of CBOR(certification.StatisticalRecord{})
		StatHash: []uint8{0x24, 0xee, 0x26, 0xf4, 0xaa, 0x45, 0x48, 0x5f, 0x53, 0xaa, 0xb4, 0x77, 0x57, 0xd0, 0xb9, 0x71, 0x99, 0xa3, 0xd9, 0x5f, 0x50, 0xcb, 0x97, 0x9c, 0x38, 0x3b, 0x7e, 0x50, 0x24, 0xf9, 0x21, 0xff},
	}

	fees := map[string]uint64{}
	for _, v := range nodes {
		fees[v] = 0
	}
	h := hash.New(gocrypto.SHA256.New())
	h.WriteRaw(types.RawCBOR{0xA0}) // empty map
	h.Write(fees)

	feeHash, err := h.Sum()
	require.NoError(t, err)
	tr.FeeHash = feeHash
	return tr
}
