package partition

import (
	"context"
	gocrypto "crypto"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	pgenesis "github.com/alphabill-org/alphabill/internal/partition/genesis"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

type AlwaysValidCertificateValidator struct{}

func (c *AlwaysValidCertificateValidator) Validate(_ *types.UnicityCertificate) error {
	return nil
}

func TestNode_StartNewRoundCallsRInit(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p := RunSingleNodePartition(t, s)
	ucr := &types.UnicityCertificate{
		InputRecord: &types.InputRecord{
			RoundNumber: 2,
		},
		UnicitySeal: &types.UnicitySeal{
			RootChainRoundNumber: 1,
			Hash:                 zeroHash,
		},
	}
	p.partition.startNewRound(context.Background(), ucr)
	require.Equal(t, uint64(1), s.BeginBlockCountDelta)
}

func TestNode_NodeStartTest(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	// node starts in init state
	require.Equal(t, initializing, tp.partition.status.Load())
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitShortTick)
	// simulate no response, but monitor timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitShortTick)
	// while no response is received a retry is triggered on each timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitShortTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(tp.partition.luc.Load())
	// node is initiated
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_NodeStartWithRecoverStateFromDB(t *testing.T) {
	db := memorydb.New()
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{}
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(db))
	genesisBlock := &types.Block{
		Header: &types.Header{
			SystemID:   tp.nodeDeps.genesis.SystemDescriptionRecord.SystemIdentifier,
			ProposerID: "test",
		},
		Transactions:       []*types.TransactionRecord{},
		UnicityCertificate: tp.nodeDeps.genesis.Certificate,
	}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2, testtransaction.NewTransactionRecord(t))
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3, testtransaction.NewTransactionRecord(t))
	require.NoError(t, db.Write(util.Uint64ToBytes(pgenesis.PartitionRoundNumber), genesisBlock))
	require.NoError(t, db.Write(util.Uint64ToBytes(2), newBlock2))
	require.NoError(t, db.Write(util.Uint64ToBytes(3), newBlock3))
	// add transactions from block 4 as pending block
	proposal := &pendingBlockProposal{
		RoundNumber:    newBlock4.GetRoundNumber(),
		ProposerNodeId: newBlock4.GetProposerID(),
		PrevHash:       newBlock3.UnicityCertificate.InputRecord.Hash,
		StateHash:      newBlock4.UnicityCertificate.InputRecord.Hash,
		Transactions:   newBlock4.Transactions,
		StateSummary:   make([]byte, 8),
	}
	require.NoError(t, db.Write(util.Uint32ToBytes(proposalKey), proposal))
	// start node with db filled
	ctx, cancel := context.WithCancel(context.Background())
	done := StartSingleNodePartition(ctx, t, tp)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	// Ask Node for latest block
	b := tp.GetLatestBlock(t)
	require.Equal(t, uint64(3), b.GetRoundNumber())
	// Simulate UC received for block 4 - the pending block
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)
	ContainsEventType(t, tp, event.BlockFinalized)
	require.Equal(t, uint64(3), b.GetRoundNumber())
}

func TestNode_CreateBlocks(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())
	transfer := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(transfer))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	tp.CreateBlock(t)

	block1 := tp.GetLatestBlock(t)
	require.NotEmpty(t, block1.GetProposerID())
	require.True(t, ContainsTransaction(block1, transfer))

	tx1 := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTxFromRPC(tx1))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	tp.eh.Reset()
	tx2 := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(tx2))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	tp.eh.Reset()
	tp.CreateBlock(t)

	block3 := tp.GetLatestBlock(t)
	require.True(t, ContainsTransaction(block3, tx1))
	require.True(t, ContainsTransaction(block3, tx2))
	require.False(t, ContainsTransaction(block3, transfer))
}

// create non-empty block #1 -> empty block #2 -> empty block #3 -> non-empty block #4
func TestNode_SubsequentEmptyBlocksNotPersisted(t *testing.T) {
	t.SkipNow()
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	genesis := tp.GetLatestBlock(t)
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block1 := tp.GetLatestBlock(t)
	require.NotEmpty(t, block1.GetProposerID())
	require.NotEqual(t, genesis.UnicityCertificate.InputRecord.RoundNumber, block1.UnicityCertificate.InputRecord.RoundNumber)
	require.NotEqual(t, genesis.UnicityCertificate.InputRecord.BlockHash, block1.UnicityCertificate.InputRecord.BlockHash)

	// next block (empty)
	tp.CreateBlock(t)
	block2 := tp.GetLatestBlock(t) // this returns same block1 since empty block is not persisted
	require.Equal(t, block1, block2)
	// latest UC certifies empty block
	uc2 := tp.partition.luc.Load()
	require.Less(t, block2.UnicityCertificate.InputRecord.RoundNumber, uc2.InputRecord.RoundNumber)
	// hash of the latest certified empty block is zero-hash
	require.Equal(t, uc2.InputRecord.BlockHash, zeroHash)
	// state hash must stay the same as in last non-empty block
	require.Equal(t, block2.UnicityCertificate.InputRecord.Hash, uc2.InputRecord.Hash)

	// next block (empty)
	tp.CreateBlock(t)
	require.Equal(t, block1, tp.GetLatestBlock(t))
	uc3 := tp.partition.luc.Load()
	require.Less(t, uc2.InputRecord.RoundNumber, uc3.InputRecord.RoundNumber)
	require.Equal(t, uc3.InputRecord.BlockHash, zeroHash)
	require.Equal(t, block1.UnicityCertificate.InputRecord.Hash, uc3.InputRecord.Hash)

	// next block (non-empty)
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block4 := tp.GetLatestBlock(t)
	require.NotEmpty(t, block4.GetProposerID())
	require.NotEqual(t, block1, block4)
	require.NotEqual(t, block4.UnicityCertificate.InputRecord.BlockHash, zeroHash)
	require.Equal(t, block1.UnicityCertificate.InputRecord.BlockHash, block4.Header.PreviousBlockHash)
	uc4 := tp.partition.luc.Load()
	require.Equal(t, block4.UnicityCertificate, uc4)
	require.Equal(t, block1.UnicityCertificate.InputRecord.Hash, uc4.InputRecord.PreviousHash)
	require.Less(t, uc3.InputRecord.RoundNumber, uc4.InputRecord.RoundNumber)
}

func TestNode_HandleNilUnicityCertificate(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.SubmitUnicityCertificate(nil)
	ContainsError(t, tp, "unicity certificate is nil")
}

func TestNode_HandleOlderUnicityCertificate(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	tp.SubmitUnicityCertificate(block.UnicityCertificate)
	ContainsError(t, tp, "new certificate is from older root round 1 than previous certificate 2")
}

func TestNode_StartNodeBehindRootchain_OK(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	systemId, err := types.SystemID(tp.nodeConf.GetSystemIdentifier()).Id32()
	require.NoError(t, err)
	luc, found := tp.certs[systemId]
	require.True(t, found)
	// Mock and skip some root rounds
	uc, err := tp.CreateUnicityCertificate(luc.InputRecord, luc.UnicitySeal.RootChainRoundNumber+3)
	require.NoError(t, err)

	tp.eh.Reset()
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)

	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.NewRoundStarted {
				return true
			}
		}
		return false

	}, test.WaitDuration, test.WaitTick)
}

func TestNode_CreateEmptyBlock(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, txSystem)
	block := tp.GetLatestBlock(t) // genesis block
	txSystem.Revert()             // revert the state of the tx system
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	uc2 := tp.partition.luc.Load()
	require.Equal(t, block.UnicityCertificate.InputRecord.RoundNumber+1, uc2.InputRecord.RoundNumber)
	require.Equal(t, block.Header.SystemID, uc2.UnicityTreeCertificate.SystemIdentifier)
	//require.Equal(t, blockHash, block2.PreviousBlockHash)
	uc1 := block.UnicityCertificate
	require.Equal(t, uc1.InputRecord.Hash, uc2.InputRecord.Hash)
	require.Equal(t, uc1.InputRecord.PreviousHash, uc2.InputRecord.PreviousHash)
	require.Equal(t, uc1.InputRecord.SummaryValue, uc2.InputRecord.SummaryValue)
	// with no transactions, block hashes do not change
	require.Equal(t, uc1.InputRecord.BlockHash, uc2.InputRecord.BlockHash)

	seal1 := block.UnicityCertificate.UnicitySeal
	seal2 := uc2.UnicitySeal
	require.Equal(t, seal1.RootChainRoundNumber+1, seal2.RootChainRoundNumber)
}

func TestNode_HandleEquivocatingUnicityCertificate_SameRoundDifferentIRHashes(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	block = tp.GetLatestBlock(t)
	require.NotNil(t, block)

	latestUC := tp.partition.luc.Load()
	ir := copyIR(latestUC.InputRecord)
	ir.Hash = test.RandomBytes(32)
	ir.BlockHash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(ir, latestUC.UnicitySeal.RootChainRoundNumber)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating UC, different input records for same partition round")
}

func copyIR(record *types.InputRecord) *types.InputRecord {
	return &types.InputRecord{
		PreviousHash:    slices.Clone(record.PreviousHash),
		Hash:            slices.Clone(record.Hash),
		BlockHash:       slices.Clone(record.BlockHash),
		SummaryValue:    slices.Clone(record.SummaryValue),
		RoundNumber:     record.RoundNumber,
		SumOfEarnedFees: record.SumOfEarnedFees,
	}
}

func TestNode_HandleEquivocatingUnicityCertificate_SameIRPreviousHashDifferentIRHash(t *testing.T) {
	txs := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, txs)
	genesisUC := tp.partition.luc.Load()
	tp.partition.startNewRound(context.Background(), genesisUC)
	block := tp.GetLatestBlock(t)
	txs.ExecuteCountDelta++ // so that the block is not considered empty
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	latestUC := tp.partition.luc.Load()
	ir := copyIR(latestUC.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(
		ir,
		latestUC.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating UC, different input records for same partition round")
}

// state does not change in case of no transactions in money partition
func TestNode_HandleUnicityCertificate_SameIR_DifferentBlockHash_StateReverted(t *testing.T) {
	txs := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, txs)
	genesisUC := tp.partition.luc.Load()
	tp.partition.startNewRound(context.Background(), genesisUC)
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)

	latestUC := tp.partition.luc.Load()
	require.NotEqual(t, genesisUC, latestUC)
	tp.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())
	// create a new transaction
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	// create block proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), txs.RevertCount)

	// simulate receiving repeat UC
	ir := latestUC.InputRecord.NewRepeatIR()
	uc, err := tp.CreateUnicityCertificate(
		ir,
		latestUC.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(uc)
	ContainsEventType(t, tp, event.StateReverted)
	require.Equal(t, uint64(1), txs.RevertCount)
}

func TestNode_HandleUnicityCertificate_ProposalIsNil(t *testing.T) {
	txSystem := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	tp := RunSingleNodePartition(t, txSystem)
	block := tp.GetLatestBlock(t)

	txSystem.EndBlockCount = 10000

	ir := copyIR(block.UnicityCertificate.InputRecord)
	ir.RoundNumber++
	uc, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(uc)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), txSystem.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
}

// proposal not nil
// uc.InputRecord.Hash != n.pr.StateHash
// uc.InputRecord.Hash == n.pr.PrevHash
// => UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
func TestNode_HandleUnicityCertificate_Revert(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	tp := RunSingleNodePartition(t, system)
	block := tp.GetLatestBlock(t)

	transfer := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// create block proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)

	// send repeat UC
	ir := copyIR(block.UnicityCertificate.InputRecord)
	ir.RoundNumber = ir.RoundNumber + 1
	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(repeatUC)
	ContainsEventType(t, tp, event.StateReverted)
	require.Equal(t, uint64(1), system.RevertCount)
}

// pending proposal exists
// uc.InputRecord.SumOfEarnedFees != n.pendingBlockProposal.SumOfEarnedFees
func TestNode_HandleUnicityCertificate_SumOfEarnedFeesMismatch_1(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{Fee: 1337})

	// skip UC validation
	tp.partition.unicityCertificateValidator = &AlwaysValidCertificateValidator{}

	// create the first block
	tp.CreateBlock(t)

	// send transaction that has a fee
	transferTx := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(transferTx))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	// when UC with modified IR.SumOfEarnedFees is received
	tp.SubmitT1Timeout(t)
	uc := tp.IssueBlockUC(t)
	uc.InputRecord.SumOfEarnedFees += 1
	tp.SubmitUnicityCertificate(uc)

	// then state is reverted
	ContainsEventType(t, tp, event.StateReverted)
}

func TestBlockProposal_BlockProposalIsNil(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.SubmitBlockProposal(nil)
	ContainsError(t, tp, blockproposal.ErrBlockProposalIsNil.Error())
}

func TestBlockProposal_InvalidNodeIdentifier(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "1", UnicityCertificate: block.UnicityCertificate})
	ContainsError(t, tp, "public key for id 1 not found")
}

func TestBlockProposal_InvalidBlockProposal(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	ver, err := tp.rootSigner.Verifier()
	require.NoError(t, err)
	rootTrust := map[string]crypto.Verifier{"test": ver}
	val, err := NewDefaultBlockProposalValidator(tp.nodeConf.genesis.SystemDescriptionRecord, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	tp.partition.blockProposalValidator = val

	tp.SubmitBlockProposal(&blockproposal.BlockProposal{
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: block.UnicityCertificate,
	})

	ContainsError(t, tp, "invalid system identifier")
}

func TestBlockProposal_HandleOldBlockProposal(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	tp.SubmitBlockProposal(&blockproposal.BlockProposal{
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		SystemIdentifier:   tp.nodeConf.GetSystemIdentifier(),
		UnicityCertificate: block.UnicityCertificate,
	})

	ContainsError(t, tp, "outdated block proposal for round 1, LUC round 2")
}

func TestBlockProposal_ExpectedLeaderInvalid(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{},
		WithLeaderSelector(&TestLeaderSelector{
			leader:      "12",
			currentNode: "12",
		}))
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	tp.SubmitBlockProposal(bp)

	ContainsError(t, tp, "invalid node identifier. leader from UC:")
}

func TestBlockProposal_Ok(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	tp.SubmitBlockProposal(bp)
	require.Eventually(t, RequestReceived(tp, network.ProtocolBlockCertification), test.WaitDuration, test.WaitTick)
}

func TestBlockProposal_TxSystemStateIsDifferent_sameUC(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	tp.SubmitBlockProposal(bp)
	ContainsError(t, tp, "tx system start state mismatch error, expected")
}

func TestBlockProposal_TxSystemStateIsDifferent_newUC(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	block := tp.GetLatestBlock(t)
	// create a UC for a new round
	ir := &types.InputRecord{
		Hash:         block.UnicityCertificate.InputRecord.Hash,
		PreviousHash: block.UnicityCertificate.InputRecord.PreviousHash,
		BlockHash:    block.UnicityCertificate.InputRecord.BlockHash,
		SummaryValue: block.UnicityCertificate.InputRecord.SummaryValue,
		RoundNumber:  block.UnicityCertificate.InputRecord.RoundNumber + 1,
	}
	uc, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	tp.SubmitBlockProposal(bp)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	testevent.ContainsEvent(t, tp.eh, event.StateReverted)
	require.Equal(t, recovering, tp.partition.status.Load())
}

func TestNode_GetTransactionRecord_OK(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	indexDB := memorydb.New()
	tp := RunSingleNodePartition(t, system, WithProofIndex(indexDB, 0))
	require.NoError(t, tp.partition.startNewRound(context.Background(), tp.partition.luc.Load()))
	order := testtransaction.NewTransactionOrder(t, testtransaction.WithPayloadType("test21"))
	hash := order.Hash(tp.partition.configuration.hashAlgorithm)
	require.NoError(t, tp.SubmitTx(order))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	order2 := testtransaction.NewTransactionOrder(t, testtransaction.WithPayloadType("test22"))
	hash2 := order2.Hash(tp.partition.configuration.hashAlgorithm)
	require.NoError(t, tp.SubmitTxFromRPC(order2))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)

	require.Eventually(t, func() bool {
		record, proof, err := tp.partition.GetTransactionRecord(context.Background(), hash)
		require.NoError(t, err)
		return record != nil && proof != nil
	}, test.WaitDuration, test.WaitTick)

	require.Eventually(t, func() bool {
		record, proof, err := tp.partition.GetTransactionRecord(context.Background(), hash2)
		require.NoError(t, err)
		return record != nil && proof != nil
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_GetTransactionRecord_NotFound(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system, WithProofIndex(memorydb.New(), 0))
	record, proof, err := tp.partition.GetTransactionRecord(context.Background(), test.RandomBytes(32))

	require.ErrorIs(t, err, IndexNotFound)
	require.Nil(t, record)
	require.Nil(t, proof)
}
