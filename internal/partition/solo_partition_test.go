package partition

import (
	"context"
	gocrypto "crypto"
	"reflect"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain/unicitytree"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
)

type AlwaysValidBlockProposalValidator struct{}
type AlwaysValidTransactionValidator struct{}

func (t *AlwaysValidTransactionValidator) Validate(*transaction.Transaction) error {
	return nil
}
func (t *AlwaysValidBlockProposalValidator) Validate(*blockproposal.BlockProposal, crypto.Verifier) error {
	return nil
}

type SingleNodePartition struct {
	nodeConf             *Configuration
	eventBus             *eventbus.EventBus
	store                *store.InMemoryBlockStore
	partition            *Partition
	p1Channel            <-chan interface{}
	blockProposalChannel <-chan interface{}
	rootState            *rootchain.State
	rootSigner           crypto.Signer
}

func NewSingleNodePartition(t *testing.T, txSystem txsystem.TransactionSystem) *SingleNodePartition {
	// node genesis
	nodeSigner, _ := testsig.CreateSignerAndVerifier(t)
	nodeGenesis, err := NewNodeGenesis(txSystem, WithPeerID("1"),
		WithSigner(nodeSigner),
		WithSystemIdentifier([]byte{1, 1, 1, 1}))
	if err != nil {
		t.Error(err)
	}

	// partition genesis
	partitionRecord, err := NewPartitionGenesis([]*genesis.PartitionNode{nodeGenesis}, 2500)
	if err != nil {
		t.Error(err)
	}

	// root genesis
	rootSigner, rootVerifier := testsig.CreateSignerAndVerifier(t)
	rootGenesis, partitionGenesis, err := rootchain.NewGenesis([]*genesis.PartitionRecord{partitionRecord}, rootSigner)
	if err != nil {
		t.Error(err)
	}

	// root chain
	rc, err := rootchain.NewStateFromGenesis(rootGenesis, rootSigner)
	if err != nil {
		t.Error(err)
	}
	c := &Configuration{
		T1Timeout:     999 * time.Hour,
		TrustBase:     rootVerifier,
		Signer:        nodeSigner,
		HashAlgorithm: gocrypto.SHA256,
		Genesis:       partitionGenesis[0],
	}

	leaderSelector := &TestLeaderSelector{
		leader:      peer.ID("1"),
		currentNode: peer.ID("1"),
	}
	validator, err := NewDefaultUnicityCertificateValidator(partitionRecord.SystemDescriptionRecord, rootVerifier, gocrypto.SHA256)
	if err != nil {
		t.Error(err)
	}
	bus := eventbus.New()
	s := store.NewInMemoryBlockStore()

	// partition
	p, err := New(
		context.Background(),
		txSystem,
		bus,
		leaderSelector,
		validator,
		&AlwaysValidTransactionValidator{},
		s,
		c,
	)
	if err != nil {
		t.Error(err)
	}
	p.blockProposalValidator = &AlwaysValidBlockProposalValidator{}
	p1Channel, err := bus.Subscribe(eventbus.TopicP1, 10)
	if err != nil {
		t.Error(err)
	}
	blockProposalChannel, err := bus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	if err != nil {
		t.Error(err)
	}
	partition := &SingleNodePartition{
		eventBus:             bus,
		p1Channel:            p1Channel,
		blockProposalChannel: blockProposalChannel,
		partition:            p,
		rootState:            rc,
		nodeConf:             c,
		store:                s,
		rootSigner:           rootSigner,
	}
	t.Cleanup(partition.Close)
	return partition
}

func (tp *SingleNodePartition) Close() {
	tp.partition.Close()
}

func (tp *SingleNodePartition) SubmitTx(tx *transaction.Transaction) error {
	return tp.eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: tx})
}

func (tp *SingleNodePartition) SubmitUnicityCertificate(uc *certificates.UnicityCertificate) error {
	return tp.partition.handleUnicityCertificate(uc)
}

func (tp *SingleNodePartition) SubmitBlockProposal(prop *blockproposal.BlockProposal) error {
	return tp.partition.handleBlockProposal(prop)
}

func (tp *SingleNodePartition) GetProposalTxs() []*transaction.Transaction {
	return tp.partition.GetCurrentProposal()
}

func (tp *SingleNodePartition) CreateUnicityCertificate(ir *certificates.InputRecord, roundNumber uint64, previousRoundRootHash []byte) (*certificates.UnicityCertificate, error) {
	systemIdentifier := tp.nodeConf.GetSystemIdentifier()
	sdrhash := tp.nodeConf.Genesis.SystemDescriptionRecord.Hash(gocrypto.SHA256)
	data := []*unicitytree.Data{{
		SystemIdentifier:            systemIdentifier,
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
	cert, err := ut.GetCertificate(systemIdentifier)
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

func (tp *SingleNodePartition) GetLatestBlock() (*block.Block, error) {
	return tp.store.LatestBlock()
}

func (tp *SingleNodePartition) CreateBlock() error {
	tp.partition.handleT1TimeoutEvent()
	e := <-tp.p1Channel
	p1E := e.(eventbus.P1Event)

	req := &p1.P1Request{
		SystemIdentifier: p1E.SystemIdentifier,
		NodeIdentifier:   p1E.NodeIdentifier.String(),
		RootRoundNumber:  p1E.LucRoundNumber,
		InputRecord:      p1E.InputRecord,
	}
	err := req.Sign(tp.nodeConf.Signer)
	if err != nil {
		return err
	}
	respCh := make(chan *p1.P1Response, 1)
	tp.rootState.HandleInputRequestEvent(&p1.RequestEvent{
		Req:        req,
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

func (l *TestLeaderSelector) GetLeader(seal *certificates.UnicitySeal) peer.ID {
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
		b, _ := tp.GetLatestBlock()
		return b.UnicityCertificate.UnicitySeal.RootChainRoundNumber == prevBlock.UnicityCertificate.UnicitySeal.GetRootChainRoundNumber()+1
	}
}

func ProposalContains(tp *SingleNodePartition, t *transaction.Transaction) func() bool {
	return func() bool {
		for _, tx := range tp.GetProposalTxs() {
			if reflect.DeepEqual(tx, t) {
				return true
			}
		}
		return false
	}
}

func ContainsTransaction(block *block.Block, tx *transaction.Transaction) bool {
	for _, t := range block.Transactions {
		if t == tx {
			return true
		}
	}
	return false
}
