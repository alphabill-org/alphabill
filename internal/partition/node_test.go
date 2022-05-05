package partition

import (
	gocrypto "crypto"
	"fmt"
	"strings"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	libp2p2crypto "github.com/libp2p/go-libp2p-core/crypto"
	"google.golang.org/protobuf/proto"

	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"
	testtxsystem "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"github.com/stretchr/testify/require"
)

var (
	certificateValidator = &AlwaysValidCertificateValidator{}
	txValidator          = &AlwaysValidTransactionValidator{}

	testSystemIdentifier = []byte{1, 2, 3, 4}
)

type AlwaysValidCertificateValidator struct{}

func TestNode_StartNewRoundCallsRInit(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p, _ := createNodeWithTxSystem(t, testSystemIdentifier, s)
	ucr := &certificates.UnicityCertificate{
		UnicitySeal: &certificates.UnicitySeal{
			RootChainRoundNumber: 0,
			PreviousHash:         nil,
			Hash:                 nil,
		},
	}
	p.startNewRound(ucr)
	require.Equal(t, uint64(2), s.BeginBlockCountDelta)
}

func TestNew_StartsMainLoop(t *testing.T) {
	_, eventBus := createNodeWithTxSystem(t, testSystemIdentifier, &testtxsystem.CounterTxSystem{})

	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)
	require.NoError(t, err)

	eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
}

func TestNode_HandleInvalidTxEvent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	p, eventBus := createNodeWithTxSystem(t, testSystemIdentifier, system)
	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)

	require.NoError(t, err)
	require.NoError(t, eventBus.Submit(eventbus.TopicPartitionTransaction, "invalid tx"))

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	require.Equal(t, 0, len(p.proposal))
	require.Equal(t, 0, len(p.pr.Transactions))
}

func TestNode_HandleUnicityCertificateRecordEvent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	p, eventBus := createNodeWithTxSystem(t, testSystemIdentifier, system)
	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)
	require.NoError(t, err)

	err = eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})
	require.NoError(t, err)

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	var p1Event eventbus.BlockCertificationEvent
	require.Eventually(t, func() bool { e := <-p1; p1Event = e.(eventbus.BlockCertificationEvent); return true }, test.WaitDuration, test.WaitTick)
	err = eventBus.Submit(eventbus.TopicPartitionUnicityCertificate, eventbus.UnicityCertificateEvent{
		Certificate: &certificates.UnicityCertificate{
			InputRecord: p1Event.Req.InputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier: []byte{0, 0, 0, 1},
			},
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: 2,
				PreviousHash:         nil,
				Hash:                 nil,
			},
		}},
	)

	require.NoError(t, err)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	height, err := p.blockStore.Height()
	require.NoError(t, err)
	require.Equal(t, uint64(2), height)

	latestBlock := p.blockStore.LatestBlock()
	require.Equal(t, uint64(2), latestBlock.BlockNumber)
	require.Equal(t, 1, len(latestBlock.Transactions))
}

func TestNode_CreateBlocks(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	transfer := testtransaction.RandomBillTransfer()

	block1 := tp.GetLatestBlock()
	require.NoError(t, tp.SubmitTx(transfer))
	require.Eventually(t, ProposalContains(tp, transfer), test.WaitDuration, test.WaitTick)

	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block1), test.WaitDuration, test.WaitTick)

	block2 := tp.GetLatestBlock()
	require.True(t, ContainsTransaction(block2, transfer))

	tx1 := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(tx1))
	require.Eventually(t, ProposalSize(tp, 1), test.WaitDuration, test.WaitTick)
	tx2 := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(tx2))
	require.Eventually(t, ProposalSize(tp, 2), test.WaitDuration, test.WaitTick)
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block2), test.WaitDuration, test.WaitTick)

	block3 := tp.GetLatestBlock()
	require.True(t, ContainsTransaction(block3, tx1))
	require.True(t, ContainsTransaction(block3, tx2))
	require.False(t, ContainsTransaction(block3, transfer))
}

func TestNode_HandleNilUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	err := tp.SubmitUnicityCertificate(nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid unicity certificate: unicity certificate is nil"))
}

func TestNode_HandleOlderUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()

	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	err := tp.SubmitUnicityCertificate(block.UnicityCertificate)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "received UC is older than LUC. uc round 1, luc round 2"))
}

func TestNode_CreateEmptyBlock(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txSystem)
	block := tp.GetLatestBlock()
	txSystem.EndBlockCount-- // revert the state of the tx system
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	block2 := tp.GetLatestBlock()
	require.Equal(t, block.BlockNumber+1, block2.BlockNumber)
	require.Equal(t, block.SystemIdentifier, block2.SystemIdentifier)
	require.Equal(t, block.Hash(gocrypto.SHA256), block2.PreviousBlockHash)
	uc1 := block.UnicityCertificate
	uc2 := block2.UnicityCertificate
	require.Equal(t, uc1.InputRecord.Hash, uc2.InputRecord.Hash)
	require.Equal(t, uc1.InputRecord.PreviousHash, uc2.InputRecord.PreviousHash)
	require.Equal(t, uc1.InputRecord.SummaryValue, uc2.InputRecord.SummaryValue)
	require.NotEqual(t, uc1.InputRecord.BlockHash, uc2.InputRecord.BlockHash)

	seal1 := block.UnicityCertificate.UnicitySeal
	seal2 := block2.UnicityCertificate.UnicitySeal
	require.Equal(t, seal1.RootChainRoundNumber+1, seal2.RootChainRoundNumber)

}

func TestNode_HandleEquivocatingUnicityCertificate_SameRoundDifferentIRHashes(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)
	block = tp.GetLatestBlock()

	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(ir, block.UnicityCertificate.UnicitySeal.RootChainRoundNumber, block.UnicityCertificate.UnicitySeal.PreviousHash)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(equivocatingUC)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "equivocating certificates: round number"))
}

func TestNode_HandleEquivocatingUnicityCertificate_SameIRPreviousHashDifferentIRHash(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	block2 := tp.GetLatestBlock()
	ir := proto.Clone(block2.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(
		ir,
		block2.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		block2.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(equivocatingUC)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "equivocating certificates. previous IR hash "))
}

func TestNode_HandleUnicityCertificate_ProposalIsNil(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txSystem)
	block := tp.GetLatestBlock()

	txSystem.EndBlockCount = 10000

	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	uc, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(uc)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNodeDoesNotHaveLatestBlock)
	require.Equal(t, uint64(0), txSystem.RevertCount)
	require.Equal(t, recovering, tp.partition.status)
}

func TestNode_HandleUnicityCertificate_Revert(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block := tp.GetLatestBlock()

	transfer := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(transfer))
	require.Eventually(t, ProposalContains(tp, transfer), test.WaitDuration, test.WaitTick)

	// create block proposal and drain channels
	tp.partition.handleT1TimeoutEvent()
	<-tp.p1Channel
	<-tp.blockProposalChannel
	require.Equal(t, uint64(0), system.RevertCount)

	// send repeat UC
	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(repeatUC)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrStateReverted)
	require.Equal(t, uint64(1), system.RevertCount)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()

	require.NoError(t, tp.SubmitTx(transfer))
	require.Eventually(t, ProposalContains(tp, transfer), test.WaitDuration, test.WaitTick)

	// prepare proposal
	tp.partition.handleT1TimeoutEvent()
	<-tp.p1Channel
	<-tp.blockProposalChannel
	require.Equal(t, uint64(0), system.RevertCount)
	// send UC with different IR hash
	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(repeatUC)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNodeDoesNotHaveLatestBlock)
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status)
}

func TestBlockProposal_BlockProposalIsNil(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	err := tp.HandleBlockProposal(nil)
	require.ErrorIs(t, err, blockproposal.ErrBlockProposalIsNil)
}

func TestBlockProposal_InvalidNodeIdentifier(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()

	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)
	err := tp.HandleBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "1", UnicityCertificate: block.UnicityCertificate})
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "public key with node id 1 not found"))
}

func TestBlockProposal_InvalidBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()

	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	val, err := NewDefaultBlockProposalValidator(tp.nodeConf.genesis.SystemDescriptionRecord, tp.nodeConf.trustBase, gocrypto.SHA256)
	require.NoError(t, err)
	tp.partition.blockProposalValidator = val
	err = tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "r", UnicityCertificate: block.UnicityCertificate})
	require.ErrorIs(t, err, p1.ErrInvalidSystemIdentifier)
}

func TestBlockProposal_HandleOldBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()

	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	err := tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "r", UnicityCertificate: block.UnicityCertificate})
	require.True(t, strings.Contains(err.Error(), "received UC is older than LUC. uc round 1, luc round 2"))
}

func TestBlockProposal_ExpectedLeaderInvalid(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)
	tp.partition.leaderSelector = &TestLeaderSelector{
		leader:      "12",
		currentNode: "12",
	}
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     "r",
		UnicityCertificate: uc,
		Transactions:       []*transaction.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	err = tp.HandleBlockProposal(bp)
	require.True(t, strings.Contains(err.Error(), "invalid node identifier. leader from UC:"))
}

func TestBlockProposal_Ok(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock()
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     "r",
		UnicityCertificate: uc,
		Transactions:       []*transaction.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	err = tp.SubmitBlockProposal(bp)
	require.NoError(t, err)
	require.Eventually(t, CertificationRequestReceived(tp), test.WaitDuration, test.WaitTick)
}

func TestBlockProposal_TxSystemStateIsDifferent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block := tp.GetLatestBlock()
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
		block.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     "r",
		UnicityCertificate: uc,
		Transactions:       []*transaction.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	err = tp.HandleBlockProposal(bp)
	require.True(t, strings.Contains(err.Error(), "invalid tx system state root. expected"))
}

func createPeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := libp2p2crypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func createNodeWithTxSystem(t *testing.T, systemIdentifier []byte, system txsystem.TransactionSystem) (*Node, *eventbus.EventBus) {
	t.Helper()
	peer := createPeer(t)
	_, pg, signer, _ := initPartitionGenesis(t, peer, systemIdentifier, system)
	p, err := New(peer, signer, system, pg, WithUnicityCertificateValidator(certificateValidator), WithTxValidator(txValidator))
	require.NoError(t, err)
	require.Equal(t, idle, p.status)
	t.Cleanup(p.Close)
	return p, p.eventbus
}

func initPartitionGenesis(
	t *testing.T,
	peer *network.Peer,
	systemIdentifier []byte,
	system txsystem.TransactionSystem,
) (*genesis.RootGenesis, *genesis.PartitionGenesis, crypto.Signer, crypto.Signer) {

	t.Helper()
	nodeSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	pn, err := NewNodeGenesis(system, WithPeerID(peer.ID()), WithSystemIdentifier(systemIdentifier), WithSigner(nodeSigner))
	require.NoError(t, err)

	pr, err := NewPartitionGenesis([]*genesis.PartitionNode{pn}, 2500)
	require.NoError(t, err)

	rootGenesis, pss, err := rootchain.NewGenesis([]*genesis.PartitionRecord{pr}, rootSigner)
	require.NoError(t, err)

	return rootGenesis, pss[0], nodeSigner, rootSigner
}

func (c *AlwaysValidCertificateValidator) Validate(_ *certificates.UnicityCertificate) error {
	return nil
}
