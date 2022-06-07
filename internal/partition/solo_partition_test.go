package partition

import (
	gocrypto "crypto"
	"reflect"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain/unicitytree"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

func (t *AlwaysValidTransactionValidator) Validate(_ txsystem.GenericTransaction) error {
	return nil
}
func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

type SingleNodePartition struct {
	nodeConf             *configuration
	eventBus             *eventbus.EventBus
	store                store.BlockStore
	partition            *Node
	p1Channel            <-chan interface{}
	blockProposalChannel <-chan interface{}
	rootState            *rootchain.State
	rootSigner           crypto.Signer
}

func NewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem) *SingleNodePartition {
	p := createPeer(t)

	key, err := p.PublicKey()
	require.NoError(t, err)
	pubKeyBytes, err := key.Raw()
	require.NoError(t, err)

	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)

	systemId := []byte{1, 1, 1, 1}
	nodeGenesis, err := NewNodeGenesis(txSystem, WithPeerID("1"),
		WithSigningKey(nodeSigner),
		WithEncryptionPubKey(pubKeyBytes),
		WithSystemIdentifier(systemId),
		WithT2Timeout(2500),
	)
	if err != nil {
		t.Error(err)
	}

	// root genesis
	rootSigner, _ := testsig.CreateSignerAndVerifier(t)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)

	rootGenesis, partitionGenesis, err := rootchain.NewGenesisFromPartitionNodes([]*genesis.PartitionNode{nodeGenesis}, rootSigner, encPubKey)
	if err != nil {
		t.Error(err)
	}

	// root chain
	rc, err := rootchain.NewStateFromGenesis(rootGenesis, rootSigner)
	if err != nil {
		t.Error(err)
	}
	leaderSelector := &TestLeaderSelector{
		leader:      peer.ID("1"),
		currentNode: peer.ID("1"),
	}
	// partition

	n, err := New(
		p,
		nodeSigner,
		txSystem,
		partitionGenesis[0],
		WithLeaderSelector(leaderSelector),
		WithTxValidator(&AlwaysValidTransactionValidator{}),
	)
	if err != nil {
		t.Error(err)
	}
	n.blockProposalValidator = &AlwaysValidBlockProposalValidator{}
	p1Channel, err := n.eventbus.Subscribe(eventbus.TopicP1, 10)
	if err != nil {
		t.Error(err)
	}
	blockProposalChannel, err := n.eventbus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	if err != nil {
		t.Error(err)
	}
	partition := &SingleNodePartition{
		eventBus:             n.eventbus,
		p1Channel:            p1Channel,
		blockProposalChannel: blockProposalChannel,
		partition:            n,
		rootState:            rc,
		nodeConf:             n.configuration,
		store:                n.blockStore,
		rootSigner:           rootSigner,
	}
	t.Cleanup(partition.Close)
	return partition
}

func (tp *SingleNodePartition) Close() {
	tp.partition.Close()
}

func (tp *SingleNodePartition) SubmitTx(tx *txsystem.Transaction) error {
	return tp.eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: tx})
}

func (tp *SingleNodePartition) SubmitUnicityCertificate(uc *certificates.UnicityCertificate) error {
	return tp.partition.handleUnicityCertificate(uc)
}

func (tp *SingleNodePartition) HandleBlockProposal(prop *blockproposal.BlockProposal) error {
	return tp.partition.handleBlockProposal(prop)
}

func (tp *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) error {
	return tp.eventBus.Submit(eventbus.TopicBlockProposalInput, eventbus.BlockProposalEvent{BlockProposal: prop})
}

func (tp *SingleNodePartition) GetProposalTxs() []txsystem.GenericTransaction {
	return tp.partition.proposal
}

func (tp *SingleNodePartition) CreateUnicityCertificate(ir *certificates.InputRecord, roundNumber uint64, previousRoundRootHash []byte) (*certificates.UnicityCertificate, error) {
	id := tp.nodeConf.GetSystemIdentifier()
	sdrhash := tp.nodeConf.genesis.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	data := []*unicitytree.Data{{
		SystemIdentifier:            id,
		InputRecord:                 ir,
		SystemDescriptionRecordHash: sdrhash,
	},
	}
	ut, err := unicitytree.New(gocrypto.SHA256.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	unicitySeal, err := tp.createUnicitySeal(roundNumber, previousRoundRootHash, rootHash)
	if err != nil {
		return nil, err
	}
	cert, err := ut.GetCertificate(id)
	if err != nil {
		// this should never happen. if it does then exit with panic because we cannot generate
		// unicity tree certificates.
		panic(err)
	}

	return &certificates.UnicityCertificate{
		InputRecord: ir,
		UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
			SystemIdentifier:      cert.SystemIdentifier,
			SiblingHashes:         cert.SiblingHashes,
			SystemDescriptionHash: sdrhash,
		},
		UnicitySeal: unicitySeal,
	}, nil
}

func (tp *SingleNodePartition) createUnicitySeal(roundNumber uint64, previousRoundRootHash, rootHash []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: roundNumber,
		PreviousHash:         previousRoundRootHash,
		Hash:                 rootHash,
	}
	return u, u.Sign(tp.rootSigner)
}

func (tp *SingleNodePartition) GetLatestBlock() *block.Block {
	return tp.store.LatestBlock()
}

func (tp *SingleNodePartition) CreateBlock() error {
	tp.partition.handleT1TimeoutEvent()
	e := <-tp.p1Channel
	p1E := e.(eventbus.BlockCertificationEvent)

	req := &p1.P1Request{
		SystemIdentifier: p1E.Req.SystemIdentifier,
		NodeIdentifier:   p1E.Req.NodeIdentifier,
		RootRoundNumber:  p1E.Req.RootRoundNumber,
		InputRecord:      p1E.Req.InputRecord,
	}
	err := req.Sign(tp.nodeConf.signer)
	if err != nil {
		return err
	}
	respCh := make(chan *p1.P1Response, 1)
	tp.rootState.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        p1E.Req,
		ResponseCh: respCh,
	})

	_, err = tp.rootState.CreateUnicityCertificates()
	if err != nil {
		return err
	}
	uc := <-respCh
	return tp.eventBus.Submit(eventbus.TopicPartitionUnicityCertificate, eventbus.UnicityCertificateEvent{Certificate: uc.Message})
}

type TestLeaderSelector struct {
	leader      peer.ID
	currentNode peer.ID
}

func (l *TestLeaderSelector) SelfID() peer.ID {
	return l.currentNode
}

// IsCurrentNodeLeader returns true it current node is the leader and must propose the next block.
func (l *TestLeaderSelector) IsCurrentNodeLeader() bool {
	return l.leader == l.SelfID()
}

func (l *TestLeaderSelector) UpdateLeader(seal *certificates.UnicitySeal) {
	if seal == nil {
		l.leader = ""
		return
	}
	l.leader = l.currentNode
	return
}

func (l *TestLeaderSelector) LeaderFromUnicitySeal(seal *certificates.UnicitySeal) peer.ID {
	if seal == nil {
		return ""
	}
	return l.currentNode
}

func ProposalSize(tp *SingleNodePartition, i int) func() bool {
	return func() bool {
		return len(tp.GetProposalTxs()) == i
	}
}

func NextBlockReceived(tp *SingleNodePartition, prevBlock *block.Block) func() bool {
	return func() bool {
		b := tp.GetLatestBlock()
		return b.UnicityCertificate.UnicitySeal.RootChainRoundNumber == prevBlock.UnicityCertificate.UnicitySeal.GetRootChainRoundNumber()+1
	}
}

func ProposalContains(tp *SingleNodePartition, t *txsystem.Transaction) func() bool {
	return func() bool {
		for _, tx := range tp.GetProposalTxs() {
			if reflect.DeepEqual(tx.ToProtoBuf(), t) {
				return true
			}
		}
		return false
	}
}

func ContainsTransaction(block *block.Block, tx *txsystem.Transaction) bool {
	for _, t := range block.Transactions {
		if t == tx {
			return true
		}
	}
	return false
}

func CertificationRequestReceived(tp *SingleNodePartition) func() bool {
	return func() bool {
		e := <-tp.p1Channel
		return e != nil
	}
}
