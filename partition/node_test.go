package partition

import (
	"context"
	gocrypto "crypto"
	"slices"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/partition/event"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

type AlwaysValidCertificateValidator struct{}

func (c *AlwaysValidCertificateValidator) Validate(_ *types.UnicityCertificate) error {
	return nil
}

func TestNode_StartNewRoundCallsRInit(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p := RunSingleNodePartition(t, s)
	p.WaitHandshake(t)
	p.partition.startNewRound(context.Background())
	// handshake sent us genesis UC which triggered new round and we then triggered it manually too
	require.Equal(t, uint64(2), s.BeginBlockCountDelta)
}

func TestNode_NodeStartTest(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	// node starts in init state
	require.Equal(t, initializing, tp.partition.status.Load())
	// node sends a handshake to root
	test.TryTilCountIs(t, RequestReceived(tp, network.ProtocolHandshake), 4, test.WaitShortTick)
	// simulate no response, but monitor timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake to root
	test.TryTilCountIs(t, RequestReceived(tp, network.ProtocolHandshake), 4, test.WaitShortTick)
	// while no response is received a retry is triggered on each timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake
	test.TryTilCountIs(t, RequestReceived(tp, network.ProtocolHandshake), 4, test.WaitShortTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(tp.partition.luc.Load())
	// node is initiated
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_NodeStartWithRecoverStateFromDB(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}, WithBlockStore(db))

	uc0 := tp.GetCommittedUC(t)
	newBlock1, uc1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createNewBlockOutsideNode(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, _ := createNewBlockOutsideNode(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))
	require.NoError(t, db.Write(util.Uint64ToBytes(1), newBlock1))
	require.NoError(t, db.Write(util.Uint64ToBytes(2), newBlock2))
	// add transactions from block 4 as pending block
	require.NoError(t, db.Write(util.Uint32ToBytes(proposalKey), newBlock3))
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
	rn, err := b.GetRoundNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(2), rn)
	// Simulate UC received for block 4 - the pending block
	uc4, err := getUCv1(newBlock3)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc4)
	ContainsEventType(t, tp, event.BlockFinalized)
	b = tp.GetLatestBlock(t)
	rn, err = b.GetRoundNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(3), rn)
}

func TestNode_CreateBlocks(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	tp.partition.startNewRound(context.Background())
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

	_, err := tp.partition.GetTransactionRecordProof(context.Background(), test.RandomBytes(33))
	require.ErrorIs(t, err, ErrIndexNotFound)
}

// create non-empty block #1 -> empty block #2 -> empty block #3 -> non-empty block #4
func TestNode_SubsequentEmptyBlocksNotPersisted(t *testing.T) {
	t.SkipNow()
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	genesis := tp.GetLatestBlock(t)
	tp.partition.startNewRound(context.Background())
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block1 := tp.GetLatestBlock(t)
	require.NotEmpty(t, block1.GetProposerID())
	uc, err := getUCv1(genesis)
	require.NoError(t, err)
	uc1, err := getUCv1(block1)
	require.NoError(t, err)
	require.NotEqual(t, uc.InputRecord.RoundNumber, uc1.InputRecord.RoundNumber)
	require.NotEqual(t, uc.InputRecord.BlockHash, uc1.InputRecord.BlockHash)

	// next block (empty)
	tp.CreateBlock(t)
	block2 := tp.GetLatestBlock(t) // this returns same block1 since empty block is not persisted
	require.Equal(t, block1, block2)
	// latest UC certifies empty block
	uc2 := tp.partition.luc.Load()
	block2uc, err := getUCv1(block2)
	require.NoError(t, err)
	require.Less(t, block2uc.InputRecord.RoundNumber, uc2.InputRecord.RoundNumber)
	// hash of the latest certified empty block is zero-hash
	require.Equal(t, uc2.InputRecord.BlockHash, zeroHash)
	// state hash must stay the same as in last non-empty block
	require.Equal(t, block2uc.InputRecord.Hash, uc2.InputRecord.Hash)

	// next block (empty)
	tp.CreateBlock(t)
	require.Equal(t, block1, tp.GetLatestBlock(t))
	uc3 := tp.partition.luc.Load()
	require.Less(t, uc2.InputRecord.RoundNumber, uc3.InputRecord.RoundNumber)
	require.Equal(t, uc3.InputRecord.BlockHash, zeroHash)
	block1uc, err := getUCv1(block1)
	require.NoError(t, err)
	require.Equal(t, block1uc.InputRecord.Hash, uc3.InputRecord.Hash)

	// next block (non-empty)
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block4 := tp.GetLatestBlock(t)
	require.NotEmpty(t, block4.GetProposerID())
	require.NotEqual(t, block1, block4)
	block4uc, err := getUCv1(block4)
	require.NoError(t, err)
	require.NotEqual(t, block4uc.InputRecord.BlockHash, zeroHash)
	require.Equal(t, block1uc.InputRecord.BlockHash, block4.Header.PreviousBlockHash)
	uc4 := tp.partition.luc.Load()
	require.Equal(t, block4uc, uc4)
	require.Equal(t, block1uc.InputRecord.Hash, uc4.InputRecord.PreviousHash)
	require.Less(t, uc3.InputRecord.RoundNumber, uc4.InputRecord.RoundNumber)
}

func TestNode_InvalidCertificateResponse(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	cr := &certification.CertificationResponse{
		Partition: tp.nodeConf.GetSystemIdentifier(),
		Shard:     tp.nodeConf.shardID,
	}
	tp.mockNet.Receive(cr)
	ContainsError(t, tp, "invalid CertificationResponse: UnicityTreeCertificate is unassigned")
}

func TestNode_HandleOlderUnicityCertificate(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	committedUC := tp.GetCommittedUC(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, committedUC), test.WaitDuration, test.WaitTick)

	tp.SubmitUnicityCertificate(committedUC)
	ContainsError(t, tp, "new certificate is from older root round 1 than previous certificate 2")
}

func TestNode_StartNodeBehindRootchain_OK(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	luc, found := tp.certs[tp.nodeConf.GetSystemIdentifier()]
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
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t) // genesis state
	txSystem.Revert()           // revert the state of the tx system
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc1), test.WaitDuration, test.WaitTick)

	uc2 := tp.partition.luc.Load()
	require.Equal(t, uc1.InputRecord.RoundNumber+1, uc2.InputRecord.RoundNumber)
	require.Equal(t, uc1.UnicityTreeCertificate.SystemIdentifier, uc2.UnicityTreeCertificate.SystemIdentifier)
	//require.Equal(t, blockHash, block2.PreviousBlockHash)

	require.Equal(t, uc1.InputRecord.Hash, uc2.InputRecord.Hash)
	require.Equal(t, uc1.InputRecord.PreviousHash, uc2.InputRecord.PreviousHash)
	require.Equal(t, uc1.InputRecord.SummaryValue, uc2.InputRecord.SummaryValue)
	// with no transactions, block hashes do not change
	require.Equal(t, uc1.InputRecord.BlockHash, uc2.InputRecord.BlockHash)
	require.Equal(t, uc1.UnicitySeal.RootChainRoundNumber+1, uc2.UnicitySeal.RootChainRoundNumber)
}

func TestNode_HandleEquivocatingUnicityCertificate_SameRoundDifferentIRHashes(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc1), test.WaitDuration, test.WaitTick)
	block := tp.GetLatestBlock(t)
	require.NotNil(t, block)

	uc2 := tp.GetCommittedUC(t)
	ir := copyIR(uc2.InputRecord)
	ir.Hash = test.RandomBytes(32)
	ir.BlockHash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(ir, uc2.UnicitySeal.RootChainRoundNumber)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating UC, different input records for same partition round")
}

func copyIR(record *types.InputRecord) *types.InputRecord {
	return &types.InputRecord{Version: 1,
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
	tp.WaitHandshake(t)
	tp.partition.startNewRound(context.Background())
	uc1 := tp.GetCommittedUC(t)
	txs.ExecuteCountDelta++ // so that the block is not considered empty
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc1), test.WaitDuration, test.WaitTick)

	uc2 := tp.GetCommittedUC(t)
	ir := copyIR(uc2.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(
		ir,
		uc2.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating UC, different input records for same partition round")
}

// state does not change in case of no transactions in money partition
func TestNode_HandleUnicityCertificate_SameIR_DifferentBlockHash_StateReverted(t *testing.T) {
	txs := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, txs)
	tp.WaitHandshake(t)
	genesisUC := tp.partition.luc.Load()
	tp.partition.startNewRound(context.Background())
	require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)

	latestUC := tp.partition.luc.Load()
	require.NotEqual(t, genesisUC, latestUC)
	tp.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	tp.partition.startNewRound(context.Background())
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
	uc := tp.GetCommittedUC(t)

	txSystem.EndBlockCount = 10000

	ir := copyIR(uc.InputRecord)
	ir.RoundNumber++
	uc, err := tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
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
	tp.WaitHandshake(t)
	uc := tp.GetCommittedUC(t)

	transfer := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// create block proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)

	// send repeat UC
	ir := copyIR(uc.InputRecord)
	ir.RoundNumber = ir.RoundNumber + 1
	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
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
	tp.WaitHandshake(t)

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
	uc.InputRecord = copyIR(uc.InputRecord)
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
	tp.WaitHandshake(t)
	uc := tp.GetCommittedUC(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc), test.WaitDuration, test.WaitTick)
	tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "1", UnicityCertificate: uc})
	ContainsError(t, tp, "public key for id 1 not found")
}

func TestBlockProposal_InvalidBlockProposal(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	uc := tp.GetCommittedUC(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc), test.WaitDuration, test.WaitTick)
	verifier, err := tp.rootSigner.Verifier()
	require.NoError(t, err)
	rootTrust := trustbase.NewTrustBase(t, verifier)
	val, err := NewDefaultBlockProposalValidator(tp.nodeConf.genesis.PartitionDescription, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	tp.partition.blockProposalValidator = val

	tp.SubmitBlockProposal(&blockproposal.BlockProposal{
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc,
	})

	ContainsError(t, tp, "invalid system identifier")
}

func TestBlockProposal_HandleOldBlockProposal(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	uc := tp.GetCommittedUC(t)
	transfer := testtransaction.NewTransactionOrder(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, uc), test.WaitDuration, test.WaitTick)

	tp.SubmitBlockProposal(&blockproposal.BlockProposal{
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		Partition:          tp.nodeConf.GetSystemIdentifier(),
		UnicityCertificate: uc,
	})

	ContainsError(t, tp, "outdated block proposal for round 0, LUC round 1")
}

func TestBlockProposal_ExpectedLeaderInvalid(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	uc1 := tp.GetCommittedUC(t)
	uc2, err := tp.CreateUnicityCertificate(
		uc1.InputRecord,
		uc1.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		Partition:          uc2.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc2,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	tp.SubmitBlockProposal(bp)

	ContainsError(t, tp, "invalid node identifier. leader from UC:")
}

func TestBlockProposal_Ok(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	uc2, err := tp.CreateUnicityCertificate(
		uc1.InputRecord,
		uc1.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		Partition:          uc2.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc2,
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
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	uc2, err := tp.CreateUnicityCertificate(
		uc1.InputRecord,
		uc1.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		Partition:          uc2.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc2,
		Transactions:       []*types.TransactionRecord{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	tp.SubmitBlockProposal(bp)
	ContainsError(t, tp, "transaction system start state mismatch error, expected")
}

func TestBlockProposal_TxSystemStateIsDifferent_newUC(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	// create a UC for a new round
	ir := &types.InputRecord{Version: 1,
		Hash:         uc1.InputRecord.Hash,
		PreviousHash: uc1.InputRecord.PreviousHash,
		BlockHash:    uc1.InputRecord.BlockHash,
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  uc1.InputRecord.RoundNumber + 1,
	}
	uc2, err := tp.CreateUnicityCertificate(
		ir,
		uc1.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	bp := &blockproposal.BlockProposal{
		Partition:          uc2.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peerConf.ID.String(),
		UnicityCertificate: uc2,
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
	indexDB, err := memorydb.New()
	require.NoError(t, err)
	tp := RunSingleNodePartition(t, system, WithProofIndex(indexDB, 0))
	tp.WaitHandshake(t)
	require.NoError(t, tp.partition.startNewRound(context.Background()))
	txo := testtransaction.NewTransactionOrder(t, testtransaction.WithTransactionType(21))
	hash := txo.Hash(tp.partition.configuration.hashAlgorithm)
	require.NoError(t, tp.SubmitTx(txo))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	txo2 := testtransaction.NewTransactionOrder(t, testtransaction.WithTransactionType(22))
	hash2 := txo2.Hash(tp.partition.configuration.hashAlgorithm)
	require.NoError(t, tp.SubmitTxFromRPC(txo2))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)

	require.Eventually(t, func() bool {
		proof, err := tp.partition.GetTransactionRecordProof(context.Background(), hash)
		require.NoError(t, err)
		return proof != nil
	}, test.WaitDuration, test.WaitTick)

	require.Eventually(t, func() bool {
		proof, err := tp.partition.GetTransactionRecordProof(context.Background(), hash2)
		require.NoError(t, err)
		return proof != nil
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_GetTransactionRecord_NotFound(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	db, err := memorydb.New()
	require.NoError(t, err)
	tp := RunSingleNodePartition(t, system, WithProofIndex(db, 0))
	proof, err := tp.partition.GetTransactionRecordProof(context.Background(), test.RandomBytes(32))
	require.ErrorIs(t, err, ErrIndexNotFound)
	require.Nil(t, proof)
}
