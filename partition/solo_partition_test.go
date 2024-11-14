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
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition/event"
	consensustypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

type SingleNodePartition struct {
	nodeConf   *configuration
	store      keyvaluedb.KeyValueDB
	partition  *Node
	nodeDeps   *partitionStartupDependencies
	rootRound  uint64
	certs      map[types.PartitionID]*types.UnicityCertificate
	rootSigner crypto.Signer
	mockNet    *testnetwork.MockNet
	eh         *testevent.TestEventHandler
	obs        Observability
}

type partitionStartupDependencies struct {
	peerConf    *network.PeerConfiguration
	txSystem    txsystem.TransactionSystem
	nodeSigner  crypto.Signer
	genesis     *genesis.PartitionGenesis
	trustBase   types.RootTrustBase
	network     ValidatorNetwork
	nodeOptions []NodeOption
}

func (t *AlwaysValidTransactionValidator) Validate(_ *types.TransactionOrder, _ uint64) error {
	return nil
}

func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

func newSingleValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return newSingleNodePartition(t, txSystem, true, nodeOptions...)
}

func newSingleNonValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return newSingleNodePartition(t, txSystem, false, nodeOptions...)
}

func newSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, validator bool, nodeOptions ...NodeOption) *SingleNodePartition {
	peerConf := createPeerConfiguration(t, validator)
	pdr := types.PartitionDescriptionRecord{Version: 1, NetworkIdentifier: 5, PartitionIdentifier: 0x01010101, TypeIdLen: 8, UnitIdLen: 256, T2Timeout: 2500 * time.Millisecond}
	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)
	nodeGenesis, err := NewNodeGenesis(
		// Should actually create the genesis state before the
		// txSystem and start the txSystem with it. Works like
		// this if the txSystem has empty state as well.
		state.NewEmptyState(),
		pdr,
		WithPeerID(peerConf.ID),
		WithSigningKey(nodeSigner),
		WithEncryptionPubKey(peerConf.KeyPair.PublicKey),
	)
	require.NoError(t, err)

	// root genesis
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{nodeGenesis})
	require.NoError(t, err)
	rootGenesis, partitionGenesis, err := rootgenesis.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)
	trustBase, err := rootGenesis.GenerateTrustBase()
	require.NoError(t, err)

	require.NoError(t, txSystem.Commit(partitionGenesis[0].Certificate))

	// root state
	var certs = make(map[types.PartitionID]*types.UnicityCertificate)
	for _, partition := range rootGenesis.Partitions {
		certs[partition.GetSystemDescriptionRecord().GetPartitionIdentifier()] = partition.Certificate
	}

	net := testnetwork.NewMockNetwork(t)

	// allows restarting the node
	deps := &partitionStartupDependencies{
		peerConf:    peerConf,
		txSystem:    txSystem,
		nodeSigner:  nodeSigner,
		genesis:     partitionGenesis[0],
		trustBase:   trustBase,
		network:     net,
		nodeOptions: nodeOptions,
	}

	obs := testobserve.Default(t)
	partition := &SingleNodePartition{
		nodeDeps:   deps,
		rootRound:  rootGenesis.GetRoundNumber(),
		certs:      certs,
		rootSigner: rootSigner,
		mockNet:    net,
		eh:         &testevent.TestEventHandler{},
		obs:        observability.WithLogger(obs, obs.Logger().With(logger.NodeID(peerConf.ID))),
		nodeConf:   &configuration{},
	}
	return partition
}

func StartSingleNodePartition(ctx context.Context, t *testing.T, p *SingleNodePartition) chan struct{} {
	// partition node
	require.NoError(t, p.newNode(), "failed to init partition node")
	done := make(chan struct{})
	go func() {
		require.ErrorIs(t, p.partition.Run(ctx), context.Canceled)
		close(done)
	}()
	return done
}

func runSingleValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return runSingleNodePartition(t, txSystem, true, nodeOptions...)
}

func runSingleNonValidatorNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, nodeOptions ...NodeOption) *SingleNodePartition {
	return runSingleNodePartition(t, txSystem, false, nodeOptions...)
}

func runSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem, validator bool, nodeOptions ...NodeOption) *SingleNodePartition {
	ctx, cancel := context.WithCancel(context.Background())

	partition := newSingleNodePartition(t, txSystem, validator, nodeOptions...)
	done := StartSingleNodePartition(ctx, t, partition)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	return partition
}

func (sn *SingleNodePartition) newNode() error {
	n, err := NewNode(
		context.Background(),
		sn.nodeDeps.peerConf,
		sn.nodeDeps.nodeSigner,
		sn.nodeDeps.txSystem,
		sn.nodeDeps.genesis,
		sn.nodeDeps.trustBase,
		sn.nodeDeps.network,
		sn.obs,
		append([]NodeOption{
			WithT1Timeout(100 * time.Minute),
			WithTxValidator(&AlwaysValidTransactionValidator{}),
			WithEventHandler(sn.eh.HandleEvent, 100),
			WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		}, sn.nodeDeps.nodeOptions...)...,
	)
	if err != nil {
		return err
	}
	sn.partition = n
	sn.nodeConf = n.configuration
	sn.store = n.blockStore
	return nil
}

func (sn *SingleNodePartition) SubmitTx(tx *types.TransactionOrder) error {
	sn.mockNet.AddTransaction(context.Background(), tx)
	return nil
}

func (sn *SingleNodePartition) SubmitTxFromRPC(tx *types.TransactionOrder) error {
	_, err := sn.partition.SubmitTx(context.Background(), tx)
	return err
}

/*
ReceiveCertResponse builds UC and TR based on given input and "sends" them as CertificationResponse to the node.
*/
func (sn *SingleNodePartition) ReceiveCertResponse(t *testing.T, ir *types.InputRecord, roundNumber uint64) {
	uc, tr, err := sn.CreateUnicityCertificateTR(ir, roundNumber)
	if err != nil {
		t.Fatalf("creating UC and TR: %v", err)
	}

	sn.mockNet.Receive(&certification.CertificationResponse{
		Partition: sn.nodeConf.GetPartitionIdentifier(),
		Shard:     sn.nodeConf.shardID,
		Technical: tr,
		UC:        *uc,
	})
}

/*
SubmitUnicityCertificate wraps the UC into CertificationResponse and sends it to the node.
*/
func (sn *SingleNodePartition) SubmitUnicityCertificate(t *testing.T, uc *types.UnicityCertificate) {
	cr := &certification.CertificationResponse{
		Partition: sn.nodeConf.GetPartitionIdentifier(),
		Shard:     sn.nodeConf.shardID,
		UC:        *uc,
	}
	tr, err := rootgenesis.TechnicalRecord(uc.InputRecord, []string{sn.nodeDeps.peerConf.ID.String()})
	if err != nil {
		t.Fatalf("creating TechnicalRecord: %v", err)
	}
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
	// root responds with genesis
	uc := sn.certs[sn.partition.PartitionID()]
	cr := &certification.CertificationResponse{
		Partition: sn.partition.PartitionID(),
		Shard:     sn.partition.configuration.shardID,
		UC:        *uc,
	}
	tr, err := rootgenesis.TechnicalRecord(uc.InputRecord, []string{sn.nodeDeps.peerConf.ID.String()})
	if err != nil {
		t.Fatalf("creating TechnicalRecord: %v", err)
	}
	cr.Technical = tr
	if err := cr.IsValid(); err != nil {
		t.Errorf("invalid CertRsp: %v", err)
	}
	if err := sn.partition.handleMessage(context.Background(), cr); err != nil {
		t.Errorf("sending handshake response to the node: %v", err)
	}
}

func (sn *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) {
	sn.mockNet.Receive(prop)
}

func (sn *SingleNodePartition) CreateUnicityCertificate(ir *types.InputRecord, roundNumber uint64) (*types.UnicityCertificate, error) {
	tr, err := rootgenesis.TechnicalRecord(ir, []string{sn.nodeDeps.peerConf.ID.String()})
	if err != nil {
		return nil, fmt.Errorf("creating TechnicalRecord: %w", err)
	}
	trHash, err := tr.Hash()
	if err != nil {
		return nil, fmt.Errorf("calculating TechnicalRecord hash: %w", err)
	}

	pdr := sn.nodeDeps.genesis.PartitionDescription
	pdrHash := pdr.Hash(gocrypto.SHA256)
	sTree, err := types.CreateShardTree(pdr.Shards, []types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("creating shard tree: %w", err)
	}
	stCert, err := sTree.Certificate(types.ShardID{})
	if err != nil {
		return nil, fmt.Errorf("creating shard tree certificate: %w", err)
	}

	data := []*types.UnicityTreeData{{
		Partition:     pdr.PartitionIdentifier,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       pdrHash,
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.RootHash()
	unicitySeal, err := sn.createUnicitySeal(roundNumber, rootHash)
	if err != nil {
		return nil, err
	}
	cert, err := ut.Certificate(pdr.PartitionIdentifier)
	if err != nil {
		return nil, fmt.Errorf("creating UnicityTreeCertificate: %w", err)
	}

	return &types.UnicityCertificate{
		Version:                1,
		InputRecord:            ir,
		TRHash:                 trHash,
		ShardTreeCertificate:   stCert,
		UnicityTreeCertificate: cert,
		UnicitySeal:            unicitySeal,
	}, nil
}

func (sn *SingleNodePartition) CreateUnicityCertificateTR(ir *types.InputRecord, roundNumber uint64) (*types.UnicityCertificate, certification.TechnicalRecord, error) {
	tr, err := rootgenesis.TechnicalRecord(ir, []string{sn.nodeDeps.peerConf.ID.String()})
	if err != nil {
		return nil, tr, fmt.Errorf("creating TechnicalRecord: %w", err)
	}
	trHash, err := tr.Hash()
	if err != nil {
		return nil, tr, fmt.Errorf("calculating TechnicalRecord hash: %w", err)
	}

	pdr := sn.nodeDeps.genesis.PartitionDescription
	pdrHash := pdr.Hash(gocrypto.SHA256)
	sTree, err := types.CreateShardTree(pdr.Shards, []types.ShardTreeInput{{Shard: types.ShardID{}, IR: ir, TRHash: trHash}}, gocrypto.SHA256)
	if err != nil {
		return nil, tr, fmt.Errorf("creating shard tree: %w", err)
	}
	stCert, err := sTree.Certificate(types.ShardID{})
	if err != nil {
		return nil, tr, fmt.Errorf("creating shard tree certificate: %w", err)
	}

	data := []*types.UnicityTreeData{{
		Partition:     pdr.PartitionIdentifier,
		ShardTreeRoot: sTree.RootHash(),
		PDRHash:       pdrHash,
	}}
	ut, err := types.NewUnicityTree(gocrypto.SHA256, data)
	if err != nil {
		return nil, tr, err
	}
	rootHash := ut.RootHash()
	unicitySeal, err := sn.createUnicitySeal(roundNumber, rootHash)
	if err != nil {
		return nil, tr, err
	}
	cert, err := ut.Certificate(pdr.PartitionIdentifier)
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
	return u, u.Sign("test", sn.rootSigner)
}

func (sn *SingleNodePartition) GetCommittedUC(t *testing.T) *types.UnicityCertificate {
	uc := sn.nodeDeps.txSystem.CommittedUC()
	require.NotNil(t, uc)
	return uc
}

func (sn *SingleNodePartition) GetLatestBlock(t *testing.T) *types.Block {
	dbIt := sn.store.Last()
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
	luc, found := sn.certs[req.Partition]
	require.True(t, found)
	require.NoError(t, consensustypes.CheckBlockCertificationRequest(req, luc))
	uc, err := sn.CreateUnicityCertificate(req.InputRecord, sn.rootRound+1)
	require.NoError(t, err)
	// update state
	sn.rootRound = uc.UnicitySeal.RootChainRoundNumber
	sn.certs[req.Partition] = uc
	return uc
}

func (sn *SingleNodePartition) SubmitT1Timeout(t *testing.T) {
	sn.eh.Reset()
	sn.partition.handleT1TimeoutEvent(context.Background())
	require.Eventually(t, func() bool {
		return len(sn.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
}

func (sn *SingleNodePartition) SubmitMonitorTimeout(t *testing.T) {
	t.Helper()
	sn.eh.Reset()
	sn.partition.handleMonitoring(context.Background(), time.Now().Add(-3*sn.nodeConf.GetT2Timeout()), time.Now())
}

func createPeerConfiguration(t *testing.T, validator bool) *network.PeerConfiguration {
	// fake validator, so that network 'send' requests don't fail
	_, fakeValidatorPubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)
	fakeValidatorID, err := peer.IDFromPublicKey(fakeValidatorPubKey)
	require.NoError(t, err)

	privKey, pubKey, err := p2pcrypto.GenerateSecp256k1Key(rand.Reader)
	require.NoError(t, err)

	privKeyBytes, err := privKey.Raw()
	require.NoError(t, err)

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(t, err)

	peerID, err := peer.IDFromPublicKey(pubKey)
	require.NoError(t, err)

	validators := []peer.ID{fakeValidatorID}
	if validator {
		validators = append(validators, peerID)
	}

	peerConf, err := network.NewPeerConfiguration(
		"/ip4/127.0.0.1/tcp/0",
		nil,
		&network.PeerKeyPair{PublicKey: pubKeyBytes, PrivateKey: privKeyBytes},
		nil,
		// Need to also add peerID to make it a validator node.
		validators,
	)
	require.NoError(t, err)

	return peerConf
}

func NextBlockReceived(t *testing.T, tp *SingleNodePartition, committedUC *types.UnicityCertificate) func() bool {
	t.Helper()
	return func() bool {
		// Empty blocks are not persisted, assume new block is received if new last UC round is bigger than block UC round
		// NB! it could also be that repeat UC is received
		return tp.partition.luc.Load().GetRoundNumber() > committedUC.GetRoundNumber()
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
