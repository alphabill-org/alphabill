package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"google.golang.org/protobuf/proto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	testtxsystem "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"github.com/stretchr/testify/require"
)

var (
	certificateValidator = &AlwaysValidCertificateValidator{}
	txValidator          = &AlwaysValidTransactionValidator{}
)

type AlwaysValidCertificateValidator struct{}

func TestPartition_StartNewRoundCallsRInit(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p, _ := createTestPartitionWithTxSystem(t, s, testConfiguration(t, s))
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

func TestNewPartition_NotOk(t *testing.T) {
	type args struct {
		ctx            context.Context
		txSystem       txsystem.TransactionSystem
		eb             *eventbus.EventBus
		leaderSelector LeaderSelector
		ucrValidator   UnicityCertificateValidator
		txValidator    TxValidator
		configuration  *Configuration
		blockStore     store.BlockStore
	}
	tests := []struct {
		name   string
		args   args
		err    error
		errStr string
	}{
		{
			name: "ctx is nil",
			args: args{
				ctx:            nil,
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration: &Configuration{
					T1Timeout:     time.Second,
					TrustBase:     nil,
					Signer:        nil,
					HashAlgorithm: 0,
					Genesis:       nil},
			},
			err: ErrCtxIsNil,
		},
		{
			name: "tx system is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       nil,
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
			},
			err: ErrTxSystemIsNil,
		},
		{
			name: "eventbus is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             nil,
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
			},
			err: ErrEventBusIsNil,
		},
		{
			name: "leader selector is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: nil,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
			},
			err: ErrLeaderSelectorIsNil,
		},
		{
			name: "certificate validator is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   nil,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
			},
			err: ErrUnicityCertificateValidatorIsNil,
		},
		{
			name: "tx validator is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    nil,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
			},
			err: ErrTxValidatorIsNil,
		},
		{
			name: "configuration is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration:  nil,
			},
			err: ErrPartitionConfigurationIsNil,
		},
		{
			name: "genesis is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration: &Configuration{
					T1Timeout:     2500,
					HashAlgorithm: gocrypto.SHA256,
					Genesis:       nil,
				},
			},
			err: ErrGenesisIsNil,
		},
		{
			name: "invalid partition genesis file",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     store.NewInMemoryBlockStore(),
				configuration: &Configuration{
					T1Timeout:     2500,
					HashAlgorithm: gocrypto.SHA256,
					Genesis:       &genesis.PartitionGenesis{},
				},
			},
			errStr: "invalid root partition genesis file",
		},
		{
			name: "block store is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &testtxsystem.CounterTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: createLeaderSelector(t),
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConfiguration(t, &testtxsystem.CounterTxSystem{}),
				blockStore:     nil,
			},
			err: ErrBlockStoreIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.ctx, tt.args.txSystem, tt.args.eb, tt.args.leaderSelector, tt.args.ucrValidator, tt.args.txValidator, tt.args.blockStore, tt.args.configuration)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.errStr))
			}
			require.Nil(t, got)
		})
	}
}

func createLeaderSelector(t *testing.T) *DefaultLeaderSelector {
	t.Helper()
	selector, err := NewDefaultLeaderSelector(createPeer(t), eventbus.New())
	require.NoError(t, err)
	return selector
}

func TestNew_InvalidGenesisRootHash(t *testing.T) {
	eb := eventbus.New()
	selector, err := NewDefaultLeaderSelector(createPeer(t), eb)
	require.NoError(t, err)
	conf := testConfiguration(t, &testtxsystem.CounterTxSystem{})

	system := &testtxsystem.CounterTxSystem{}
	system.InitCount = 100

	p, err := New(
		context.Background(),
		system,
		eb,
		selector,
		certificateValidator,
		txValidator,
		store.NewInMemoryBlockStore(),
		conf,
	)
	require.Nil(t, p)
	require.ErrorIs(t, err, ErrInvalidRootHash)
}

func TestNew_InvalidSummaryValue(t *testing.T) {
	eb := eventbus.New()
	selector, err := NewDefaultLeaderSelector(createPeer(t), eb)
	require.NoError(t, err)
	conf := testConfiguration(t, &testtxsystem.CounterTxSystem{})

	system := &testtxsystem.CounterTxSystem{}
	system.SummaryValue = 100

	p, err := New(
		context.Background(),
		system,
		eb,
		selector,
		certificateValidator,
		txValidator,
		store.NewInMemoryBlockStore(),
		conf,
	)
	require.Nil(t, p)
	require.ErrorIs(t, err, ErrInvalidSummaryValue)
}

func TestNew_StartsMainLoop(t *testing.T) {
	_, eventBus := createPartitionWithDefaultTxSystem(t)

	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)
	require.NoError(t, err)

	eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
}

func TestPartition_HandleInvalidTxEvent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	conf := testConfiguration(t, system)
	p, eventBus := createTestPartitionWithTxSystem(t, system, conf)
	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)

	require.NoError(t, err)
	eventBus.Submit(eventbus.TopicPartitionTransaction, "invalid tx")

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	require.Equal(t, 0, len(p.proposal))
	require.Equal(t, 0, len(p.pr.Transactions))
}

func TestPartition_HandleUnicityCertificateRecordEvent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	partitionConf := testConfiguration(t, system)
	p, eventBus := createTestPartitionWithTxSystem(t, system, partitionConf)
	proposal, err := eventBus.Subscribe(eventbus.TopicBlockProposalOutput, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(eventbus.TopicP1, 10)
	require.NoError(t, err)

	err = eventBus.Submit(eventbus.TopicPartitionTransaction, eventbus.TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})
	require.NoError(t, err)

	require.Eventually(t, func() bool { <-proposal; return true }, test.WaitDuration, test.WaitTick)
	var p1Event eventbus.P1Event
	require.Eventually(t, func() bool { e := <-p1; p1Event = e.(eventbus.P1Event); return true }, test.WaitDuration, test.WaitTick)
	err = eventBus.Submit(eventbus.TopicPartitionUnicityCertificate, eventbus.UnicityCertificateEvent{
		Certificate: &certificates.UnicityCertificate{
			InputRecord: p1Event.InputRecord,
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

	latestBlock, _ := p.blockStore.LatestBlock()
	require.Equal(t, uint64(2), latestBlock.BlockNumber)
	require.Equal(t, 1, len(latestBlock.Transactions))
}

func TestPartition_CreateBlocks(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	transfer := testtransaction.RandomBillTransfer()

	block1, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.NoError(t, tp.SubmitTx(transfer))
	require.Eventually(t, ProposalContains(tp, transfer), test.WaitDuration, test.WaitTick)

	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block1), test.WaitDuration, test.WaitTick)

	block2, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.True(t, ContainsTransaction(block2, transfer))

	tx1 := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(tx1))
	require.Eventually(t, ProposalSize(tp, 1), test.WaitDuration, test.WaitTick)
	tx2 := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(tx2))
	require.Eventually(t, ProposalSize(tp, 2), test.WaitDuration, test.WaitTick)
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block2), test.WaitDuration, test.WaitTick)

	block3, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.True(t, ContainsTransaction(block3, tx1))
	require.True(t, ContainsTransaction(block3, tx2))
	require.False(t, ContainsTransaction(block3, transfer))
}

func TestPartition_HandleNilUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	transfer := testtransaction.RandomBillTransfer()

	_, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.NoError(t, tp.SubmitTx(transfer))
	err = tp.SubmitUnicityCertificate(nil)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "invalid unicity certificate: unicity certificate is nil"))
}

func TestPartition_HandleOlderUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	transfer := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	err = tp.SubmitUnicityCertificate(block.UnicityCertificate)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "received UC is older than LUC. uc round 1, luc round 2"))
}

func TestPartition_CreateEmptyBlock(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txSystem)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	txSystem.EndBlockCount-- // revert the state of the tx system
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	block2, err := tp.GetLatestBlock()
	require.NoError(t, err)
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

func TestPartition_HandleEquivocatingUnicityCertificate_SameRoundDifferentIRHashes(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	block, err = tp.GetLatestBlock()
	require.NoError(t, err)

	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(ir, block.UnicityCertificate.UnicitySeal.RootChainRoundNumber, block.UnicityCertificate.UnicitySeal.PreviousHash)
	require.NoError(t, err)

	err = tp.SubmitUnicityCertificate(equivocatingUC)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "equivocating certificates: round number"))
}

func TestPartition_HandleEquivocatingUnicityCertificate_SameIRPreviousHashDifferentIRHash(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	block2, err := tp.GetLatestBlock()
	require.NoError(t, err)

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

func TestPartition_HandleUnicityCertificate_ProposalIsNil(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txSystem)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)

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

func TestPartition_HandleUnicityCertificate_Revert(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
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

func TestPartition_HandleUnicityCertificate_RevertAndStartRecovery(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
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
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	transfer := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)
	err = tp.HandleBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "1", UnicityCertificate: block.UnicityCertificate})
	require.True(t, strings.Contains(err.Error(), "public key with node id 1 not found"))
}

func TestBlockProposal_InvalidBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	transfer := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)
	tp.partition.blockProposalValidator, err = NewDefaultBlockProposalValidator(tp.nodeConf.Genesis.SystemDescriptionRecord, tp.nodeConf.TrustBase, gocrypto.SHA256)
	require.NoError(t, err)
	err = tp.HandleBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "r", UnicityCertificate: block.UnicityCertificate})
	require.ErrorIs(t, err, p1.ErrInvalidSystemIdentifier)
}

func TestBlockProposal_HandleOldBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
	transfer := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(transfer))
	require.NoError(t, tp.CreateBlock())
	require.Eventually(t, NextBlockReceived(tp, block), test.WaitDuration, test.WaitTick)

	err = tp.HandleBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "r", UnicityCertificate: block.UnicityCertificate})
	require.True(t, strings.Contains(err.Error(), "received UC is older than LUC. uc round 1, luc round 2"))
}

func TestBlockProposal_ExpectedLeaderInvalid(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
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
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.Signer)
	require.NoError(t, err)
	err = tp.HandleBlockProposal(bp)
	require.True(t, strings.Contains(err.Error(), "invalid node identifier. leader from UC:"))
}

func TestBlockProposal_Ok(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
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
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.Signer)
	require.NoError(t, err)
	err = tp.SubmitBlockProposal(bp)
	require.NoError(t, err)
	require.Eventually(t, CertificationRequestReceived(tp), test.WaitDuration, test.WaitTick)
}

func TestBlockProposal_TxSystemStateIsDifferent(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	block, err := tp.GetLatestBlock()
	require.NoError(t, err)
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
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.Signer)
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

	pubKeyBytes, err := libp2pcrypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func createTestPartitionWithTxSystem(t *testing.T, system txsystem.TransactionSystem, conf *Configuration) (*Partition, *eventbus.EventBus) {
	bus := eventbus.New()
	selector, err := NewDefaultLeaderSelector(createPeer(t), bus)
	require.NoError(t, err)
	ctx := context.Background()
	p, err := New(ctx, system, bus, selector, certificateValidator, txValidator, store.NewInMemoryBlockStore(), conf)
	require.NoError(t, err)
	require.Equal(t, idle, p.status)
	t.Cleanup(p.Close)
	return p, bus
}

func createPartitionWithDefaultTxSystem(t *testing.T) (*Partition, *eventbus.EventBus) {
	s := &testtxsystem.CounterTxSystem{}
	conf := testConfiguration(t, s)
	return createTestPartitionWithTxSystem(t, s, conf)
}

func testConfiguration(t *testing.T, system txsystem.TransactionSystem) *Configuration {
	_, partitionGenesis, nodeSigner, rootSigner := initPartitionGenesis(t, system)
	rootVerifier, err := rootSigner.Verifier()
	require.NoError(t, err)
	return &Configuration{
		T1Timeout:     time.Second,
		TrustBase:     rootVerifier,
		Signer:        nodeSigner,
		HashAlgorithm: gocrypto.SHA256,
		Genesis:       partitionGenesis,
	}
}

func initPartitionGenesis(t *testing.T, system txsystem.TransactionSystem) (*genesis.RootGenesis, *genesis.PartitionGenesis, crypto.Signer, crypto.Signer) {
	t.Helper()
	nodeSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	pn, err := NewNodeGenesis(system, WithPeerID("1"), WithSystemIdentifier([]byte{0, 0, 0, 1}), WithSigner(nodeSigner))
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
