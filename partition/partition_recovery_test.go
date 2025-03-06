package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/replication"
	"github.com/alphabill-org/alphabill/partition/event"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/require"
)

func TestNode_LedgerReplicationRequestTimeout(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := runSingleValidatorNodePartition(t, system)
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	order := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(order))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &types.InputRecord{
		Version:      1,
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  uc1.InputRecord.RoundNumber + 1,
		Timestamp:    types.NewTimestamp(),
	}
	tp.ReceiveCertResponseSameEpoch(t, ir, uc1.UnicitySeal.RootChainRoundNumber+1)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	// on timeout second request is sent
	require.Eventually(t, RequestReceived(tp, network.ProtocolLedgerReplicationReq), DefaultLedgerReplicationTimeout+time.Second, test.WaitTick)

	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.BlocksNotFound,
	})
	// on LedgerReplicationResponse with not OK status, another request is sent after timeout
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
}

func TestNode_RecoveryCausedByBlockSubscriptionTimeout(t *testing.T) {
	tp := runSingleNonValidatorNodePartition(t, &testtxsystem.CounterTxSystem{},
		WithBlockSubscriptionTimeout(500*time.Millisecond))

	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == recovering
	}, test.WaitDuration, test.WaitTick)

	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.BlocksNotFound,
	})
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_differentIR(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := runSingleValidatorNodePartition(t, system)
	tp.WaitHandshake(t)
	uc1 := tp.GetCommittedUC(t)
	order := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(order))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &types.InputRecord{
		Version:      1,
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  uc1.InputRecord.RoundNumber + 1,
		Timestamp:    uc1.UnicitySeal.Timestamp,
	}
	repeatUC, _, err := tp.CreateUnicityCertificate(
		ir,
		uc1.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(t, repeatUC)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	// increment round number
	irNew := &types.InputRecord{
		Version:      1,
		PreviousHash: ir.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  ir.RoundNumber + 1,
		Timestamp:    uc1.UnicitySeal.Timestamp,
	}
	tp.ReceiveCertResponseSameEpoch(t, irNew, repeatUC.UnicitySeal.RootChainRoundNumber+1)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())
}

func TestNode_RecoverToOlderRootRound(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := runSingleValidatorNodePartition(t, system)
	uc1 := tp.GetCommittedUC(t)

	// create a new block with a new UC which node does not know about
	uc2Block, uc2 := createSameEpochBlock(t, tp, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}, uc1, testtransaction.NewTransactionRecord(t))

	// submit repeat UC so that node starts recovery
	tp.ReceiveCertResponseSameEpoch(t, uc2.InputRecord.NewRepeatIR(), uc2.UnicitySeal.RootChainRoundNumber+1)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure node has asked for uc2Block
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)

	// send uc2Block
	tp.eh.Reset()
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{uc2Block},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})

	// make sure node accepts uc2Block even though it already has a UC with newer root round (repeatUC)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
}

// AB-714 If before shutting down nodes managed to send certification requests,
// the proposal is valid and must be restored correctly since the latest UC will certify it
// that is, node does not need to send replication request, but instead should restore proposal and accept UC to finalize the block
func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_sameIR(t *testing.T) {
	store, err := memorydb.New()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	tp.WaitHandshake(t)

	// create new block
	tp.CreateBlock(t)

	// create new proposal and certify it (but not yet finalize the block on the partition side)
	tp.SubmitT1Timeout(t)
	uc := tp.IssueBlockUC(t)

	system := &testtxsystem.CounterTxSystem{}
	system.Commit(tp.nodeConf.shardConf.Certificate)
	tp.nodeDeps.txSystem = system

	// kill current partition node and start it a new
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("partition node didn't shut down within timeout")
	}

	ctx, cancel = context.WithCancel(context.Background())
	done = StartSingleNodePartition(ctx, t, tp)

	// block finalization
	tp.eh.Reset()
	tp.SubmitUnicityCertificate(t, uc)
	testevent.ContainsEvent(t, tp.eh, event.BlockFinalized)

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("partition node didn't shut down within timeout")
	}
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store, err := memorydb.New()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	tp.WaitHandshake(t)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background())
	require.NoError(t, tp.SubmitTxFromRPC(testtransaction.NewTransactionOrder(t)))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)

	// create new block
	tp.CreateBlock(t)
	tp.partition.startNewRound(context.Background())

	// create new proposal and certify it (but not yet finalize the block on the partition side)
	tp.SubmitT1Timeout(t)
	uc := tp.IssueBlockUC(t)
	bl := tp.GetLatestBlock(t)
	latestRound, err := bl.GetRoundNumber()
	require.NoError(t, err)

	// now assume while the node was offline, other validators produced several new blocks, all empty
	// that is, round number has been incremented, but the state hash is the same
	uc.InputRecord.RoundNumber += 5
	uc, _, err = tp.CreateUnicityCertificate(
		uc.InputRecord,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	// submit UC, node must recover the gap of 5 blocks
	tp.SubmitUnicityCertificate(t, uc)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	require.Equal(t, recovering, tp.partition.status.Load())
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, latestRound+1, msg.BeginBlockNumber)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_noPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store, err := memorydb.New()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	tp.WaitHandshake(t)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background())

	// create new block
	tp.CreateBlock(t)
	tp.partition.startNewRound(context.Background())

	bl := tp.GetLatestBlock(t)
	uc, err := getUCv1(bl)
	require.NoError(t, err)
	committedRound := uc.GetRoundNumber()

	// now assume while the node was offline, other validators produced several new blocks, all empty
	// that is, round number has been incremented, but the state hash is the same
	uc.InputRecord.RoundNumber += 5
	uc, _, err = tp.CreateUnicityCertificate(
		uc.InputRecord,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	// submit UC, node must not start recovery as the submitted UC is a successor of the committed UC
	tp.SubmitUnicityCertificate(t, uc)
	require.Eventually(t, func() bool {
		// No recovery, node started a new round
		return tp.partition.currentRoundNumber() == uc.GetRoundNumber()+1
	}, test.WaitDuration, test.WaitTick)

	// Create an UC that is not a successor of previous UC
	ir := &types.InputRecord{
		Version:         1,
		Epoch:           0,
		RoundNumber:     uc.GetRoundNumber() + 5,
		Hash:            test.RandomBytes(32),
		PreviousHash:    test.RandomBytes(32),
		BlockHash:       test.RandomBytes(32),
		SummaryValue:    []byte{0},
		SumOfEarnedFees: 1,
		Timestamp:       1,
	}
	uc, _, err = tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	// submit UC, node must start recovery as the submitted UC is not a successor of the committed UC
	tp.SubmitUnicityCertificate(t, uc)
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == recovering
	}, test.WaitDuration, test.WaitTick)

	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, committedRound+1, msg.BeginBlockNumber)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_missedPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store, err := memorydb.New()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	tp.WaitHandshake(t)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background())

	// create new block
	tp.CreateBlock(t)
	tp.partition.startNewRound(context.Background())

	bl := tp.GetLatestBlock(t)
	uc, err := getUCv1(bl)
	require.NoError(t, err)
	latestRound := uc.GetRoundNumber()

	// now assume the node missed the proposal due to a network hiccup, other validators finalized _one_ empty block
	// that is, round number has been incremented, but the state hash is the same
	// since there's no proposal, the node will start a new round (the one that has been already finalized)
	ir := uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 1
	uc, _, err = tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(t, uc)
	testevent.ContainsEvent(t, tp.eh, event.NewRoundStarted)

	// okay, now let's finalize another round, node should start the recovery
	ir = uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 5
	ir.PreviousHash = test.RandomBytes(32)
	ir.Hash = ir.PreviousHash
	uc, _, err = tp.CreateUnicityCertificate(
		ir,
		uc.GetRootRoundNumber()+5,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(t, uc)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	require.Equal(t, recovering, tp.partition.status.Load())

	// let's submit another UC and make sure node updates the LUC and keeps recovering
	ir = uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 1
	tp.ReceiveCertResponseSameEpoch(t, ir, uc.UnicitySeal.RootChainRoundNumber+1)
	tp.eh.Reset()
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())

	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, latestRound+1, msg.BeginBlockNumber)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withNoProposal(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := runSingleValidatorNodePartition(t, system)
	uc1 := tp.GetCommittedUC(t)

	tp.partition.startNewRound(context.Background())

	// send new UC
	rootRound := uc1.UnicitySeal.RootChainRoundNumber
	partitionRound := uc1.InputRecord.RoundNumber
	sum := uc1.InputRecord.SummaryValue
	rootRound++
	partitionRound++
	newStateHash := test.RandomBytes(32)
	ir := &types.InputRecord{
		Version:      1,
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         newStateHash,
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
		Timestamp:    uc1.UnicitySeal.Timestamp,
	}
	tp.ReceiveCertResponseSameEpoch(t, ir, rootRound)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	rootRound++
	partitionRound++
	ir = &types.InputRecord{
		Version:      1,
		PreviousHash: newStateHash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
		Timestamp:    uc1.UnicitySeal.Timestamp,
	}
	tp.ReceiveCertResponseSameEpoch(t, ir, rootRound)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())
}

func TestNode_RecoverBlocks(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock3},
		FirstBlockNumber: uc3.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	ri, err := tp.partition.CurrentRoundInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(4), ri.RoundNumber)
	latestBlock := tp.GetLatestBlock(t)
	require.NoError(t, err)
	require.Equal(t, latestBlock, newBlock3)
	b, err := tp.partition.GetBlock(context.Background(), 0)
	require.ErrorContains(t, err, "node does not have block: 0, first block: 1")
	require.Nil(t, b)
	b, err = tp.partition.GetBlock(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, b, newBlock1)
	b, err = tp.partition.GetBlock(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, b, newBlock2)
	b, err = tp.partition.GetBlock(context.Background(), 3)
	require.NoError(t, err)
	require.Equal(t, b, newBlock3)
	require.Equal(t, b, latestBlock)
	// on not found nil is returned
	b, err = tp.partition.GetBlock(context.Background(), 4)
	require.NoError(t, err)
	require.Nil(t, b)
	require.EqualValues(t, 0x01010101, tp.partition.PartitionID())
}

func TestNode_RecoverBlocks_NewerUCIsReceivedDuringRecovery(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))
	// prepare a proposal
	tp.SubmitT1Timeout(t)
	// simulate root response with newer UC from round 2
	tp.SubmitUnicityCertificate(t, uc2)
	// node starts recovery
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 3 blocks, block 3 has newer UC than the node has
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2, newBlock3},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// verify that recovery is successfully completed
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	ri, err := tp.partition.CurrentRoundInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(4), ri.RoundNumber)
	latestBlock := tp.GetLatestBlock(t)
	require.Equal(t, latestBlock, newBlock3)
	require.EqualValues(t, 0x01010101, tp.partition.PartitionID())
}

func TestNode_RecoverBlocks_withEmptyBlocksChangingState(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{EndBlockChangesState: true, FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true, FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3empty, uc3 := createSameEpochBlock(t, tp, system, uc2)
	newBlock4, uc4 := createSameEpochBlock(t, tp, system, uc3, testtransaction.NewTransactionRecord(t))
	newBlock5empty, uc5 := createSameEpochBlock(t, tp, system, uc4)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc5)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock3empty, newBlock4, newBlock5empty},
		FirstBlockNumber: uc3.GetRoundNumber(),
		LastBlockNumber:  uc5.GetRoundNumber(),
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	ri, err := tp.partition.CurrentRoundInfo(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(6), ri.RoundNumber)
	latestBlock := tp.GetLatestBlock(t)
	require.NoError(t, err)
	require.Equal(t, latestBlock, newBlock5empty)
	b, err := tp.partition.GetBlock(context.Background(), 0)
	require.ErrorContains(t, err, "node does not have block: 0, first block: 1")
	require.Nil(t, b)
	b, err = tp.partition.GetBlock(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, b, newBlock1)
	b, err = tp.partition.GetBlock(context.Background(), 2)
	require.NoError(t, err)
	require.Equal(t, b, newBlock2)
	b, err = tp.partition.GetBlock(context.Background(), 3)
	require.NoError(t, err)
	require.NotNil(t, b) // newBlock3empty
	b, err = tp.partition.GetBlock(context.Background(), 4)
	require.NoError(t, err)
	require.Equal(t, b, newBlock4)
	b, err = tp.partition.GetBlock(context.Background(), 5)
	require.NoError(t, err)
	require.Equal(t, b, latestBlock)
	// on not found nil is returned
	b, err = tp.partition.GetBlock(context.Background(), 6)
	require.NoError(t, err)
	require.Nil(t, b)
	require.EqualValues(t, 0x01010101, tp.partition.PartitionID())
}

func TestNode_RecoverSkipsRequiredBlock(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{EndBlockChangesState: true})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0)
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// skip block 1 and send block 2 only
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// still recovering
	require.Equal(t, recovering, tp.partition.status.Load())
	// node is asking for missing block 2
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, msg.BeginBlockNumber, uint64(1))

	// let's give the node block 1 and 2, but skip 2
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock3},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// still recovering
	require.Equal(t, recovering, tp.partition.status.Load())
	// node is asking for missing block 3
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	msg = req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, msg.BeginBlockNumber, uint64(2))
}

func TestNode_RecoverSkipsBlocksAndSendMixedBlocks(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 duplicate blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock1},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 1 again, but also block 2 (will get discarded due to duplicate block 1)
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with block 2 and 3
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2, newBlock3},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// and now out of the blue a response with blocks 1,2 is received again
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverReceivesInvalidBlock(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))

	altBlock2 := copyBlock(t, newBlock2)
	altBlock2.Transactions = append(altBlock2.Transactions, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, altBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	require.Equal(t, uint64(2), req.Message.(*replication.LedgerReplicationRequest).BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2, newBlock3},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverReceivesInvalidBlockNoBlockProposerId(t *testing.T) {
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	ctx, cancel := context.WithCancel(context.Background())
	done := StartSingleNodePartition(ctx, t, tp)
	tp.WaitHandshake(t)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	uc0 := tp.GetCommittedUC(t)
	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	altBlock2 := copyBlock(t, newBlock2)
	altBlock2.Header.ProposerID = ""
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, altBlock2},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc2.GetRoundNumber(),
	})
	// make sure replication request is sent again and that block 3 is asked again
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	require.Equal(t, uint64(2), req.Message.(*replication.LedgerReplicationRequest).BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2, newBlock3},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverySimulateStorageFailsOnRecovery(t *testing.T) {
	// simulate storage error on two items stored in DB
	db, err := memorydb.New()
	require.NoError(t, err)
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}, WithBlockStore(db))
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
	uc0 := tp.GetCommittedUC(t)
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send all missing blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// still recovering
	require.Equal(t, recovering, tp.partition.status.Load())
	// expect a ledger replication request
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send blocks 2, 3, but set error first
	db.MockWriteError(fmt.Errorf("disk is full"))
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2, newBlock3},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// db failed to persist block 2 because disk is full, block 2 is asked again in a loop
	// expect a ledger replication request
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	require.Equal(t, uint64(2), req.Message.(*replication.LedgerReplicationRequest).BeginBlockNumber)
	// clear error and make sure node still recovers
	db.MockWriteError(nil)
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send all missing blocks 2, 3 and make sure that node now recovers
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock2, newBlock3},
		FirstBlockNumber: uc2.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverySimulateStorageFailsDuringBlockFinalizationOnUC(t *testing.T) {
	// simulate storage error on two items stored in DB
	db, err := memorydb.New()
	require.NoError(t, err)
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}, WithBlockStore(db))
	uc0 := tp.GetCommittedUC(t)
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
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(t, uc0)
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	require.Len(t, newBlock1.Transactions, 1)
	// submit transaction
	tx, err := newBlock1.Transactions[0].GetTransactionOrderV1()
	require.NoError(t, err)
	require.NoError(t, tp.SubmitTx(tx))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// simulate T1 timeout
	// bad solution, but easiest for now
	tp.partition.handleT1TimeoutEvent(ctx)
	// block proposal is sent
	require.Eventually(t, func() bool {
		return len(tp.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
	// set DB in error state
	db.MockWriteError(fmt.Errorf("disk is full"))
	// submit UC status from root
	tp.SubmitUnicityCertificate(t, uc1)
	// block is requested from other nodes
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// expect store fails and node enters recovery
	require.Equal(t, recovering, tp.partition.status.Load())
	// make sure replication request is sent
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.NotNil(t, msg)
	// make sure block 1 is asked
	require.Equal(t, uint64(1), msg.BeginBlockNumber)
	// reset error
	db.MockWriteError(nil)
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_CertificationRequestNotSentWhenProposalStoreFails(t *testing.T) {
	// simulate storage error on two items stored in DB
	db, err := memorydb.New()
	require.NoError(t, err)
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	tp := newSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}, WithBlockStore(db))
	uc0 := tp.GetCommittedUC(t)
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
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(t, uc0)
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	require.Len(t, newBlock1.Transactions, 1)
	// mock error situation, every next write will fail with error
	db.MockWriteError(fmt.Errorf("disk full"))
	tx, err := newBlock1.Transactions[0].GetTransactionOrderV1()
	require.NoError(t, err)
	require.NoError(t, tp.SubmitTx(tx))
	require.Eventually(t, func() bool {
		events := tp.eh.GetEvents()
		for _, e := range events {
			if e.EventType == event.TransactionProcessed {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
	// submit T1 timeout
	// bad solution, but easiest for now
	tp.partition.handleT1TimeoutEvent(ctx)
	// no block certification request is sent
	require.Len(t, tp.mockNet.SentMessages(network.ProtocolBlockCertification), 0)
	// make no certification request is sent, proposal is not stored
	var pr types.Block
	found, err := db.Read(util.Uint32ToBytes(proposalKey), &pr)
	require.NoError(t, err)
	require.False(t, found)
	tp.SubmitUnicityCertificate(t, uc1)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	// block is requested from other nodes
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// reset error
	db.MockWriteError(nil)
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverySendInvalidLedgerReplicationReplies(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{FixedState: mockStateStoreOK{}}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2, uc2 := createSameEpochBlock(t, tp, system, uc1, testtransaction.NewTransactionRecord(t))
	newBlock3, uc3 := createSameEpochBlock(t, tp, system, uc2, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc3)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{nil},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock := copyBlock(t, newBlock1)
	illegalBlock.Header.PartitionID = 0xFFFFFFFF
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{illegalBlock},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	illegalBlock = copyBlock(t, newBlock1)
	illegalBlock.Header.PartitionID = 0
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{illegalBlock},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock = copyBlock(t, newBlock1)
	illegalBlock.UnicityCertificate = nil
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{illegalBlock},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	// send all blocks and assume full recovery
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1, newBlock2, newBlock3},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc3.GetRoundNumber(),
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RespondToReplicationRequest(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 3, 5, 1000))
	tp.WaitHandshake(t)
	genesisBlockNumber := tp.GetCommittedUC(t).GetRoundNumber()

	tp.partition.startNewRound(context.Background())

	// generate 4 blocks with 3 tx each (but only 2 blocks will be matched and sent)
	for i := 0; i < 4; i++ {
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.Eventually(t, func() bool {
			count := 0
			for _, e := range tp.eh.GetEvents() {
				if e.EventType == event.TransactionProcessed {
					count++
				}
			}
			return count == 3
		}, test.WaitDuration, test.WaitTick)
		tp.CreateBlock(t)
		bl := tp.GetLatestBlock(t)
		require.Equal(t, 3, len(bl.Transactions))
	}
	uc, err := getUCv1(tp.GetLatestBlock(t))
	require.NoError(t, err)
	latestBlockNumber := uc.InputRecord.RoundNumber
	require.Equal(t, uint64(4), latestBlockNumber-genesisBlockNumber)

	//send replication request, it will hit tx replication limit
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: genesisBlockNumber + 1,
		PartitionID:      tp.nodeConf.GetPartitionID(),
	})

	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	//make sure response is sent
	resp := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationResp)
	require.NotNil(t, resp)
	require.IsType(t, resp.Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.Ok, resp.Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, tp.nodeDeps.peerConf.ID.String(), resp.ID.String())
	require.Equal(t, 2, len(resp.Message.(*replication.LedgerReplicationResponse).Blocks))

	tp.eh.Reset()
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	tp.partition.configuration.replicationConfig.maxReturnBlocks = 1
	//send replication request, it will hit block replication limit
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: genesisBlockNumber + 1,
		PartitionID:      tp.nodeConf.GetPartitionID(),
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	resp = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationResp)
	require.NotNil(t, resp)
	require.IsType(t, resp.Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.Ok, resp.Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, tp.nodeDeps.peerConf.ID.String(), resp.ID.String())
	require.Equal(t, 1, len(resp.Message.(*replication.LedgerReplicationResponse).Blocks))
}

func TestNode_RespondToInvalidReplicationRequest(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 3, 5, 1000))
	tp.WaitHandshake(t)
	genesisBlockNumber := tp.GetCommittedUC(t).GetRoundNumber()

	tp.partition.startNewRound(context.Background())

	// generate 4 blocks with 3 tx each (but only 2 blocks will be matched and sent)
	for i := 0; i < 4; i++ {
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.NoError(t, tp.SubmitTx(testtransaction.NewTransactionOrder(t)))
		require.Eventually(t, func() bool {
			count := 0
			for _, e := range tp.eh.GetEvents() {
				if e.EventType == event.TransactionProcessed {
					count++
				}
			}
			return count == 3
		}, test.WaitDuration, test.WaitTick)
		tp.CreateBlock(t)
		bl := tp.GetLatestBlock(t)
		require.Equal(t, 3, len(bl.Transactions))
	}
	uc, err := getUCv1(tp.GetLatestBlock(t))
	require.NoError(t, err)
	latestBlockNumber := uc.InputRecord.RoundNumber
	require.Equal(t, uint64(4), latestBlockNumber-genesisBlockNumber)
	// does not have the block 11
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 11,
		PartitionID:      tp.nodeConf.GetPartitionID(),
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	//make sure response is sent
	resp := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationResp)
	require.NotNil(t, resp)
	require.IsType(t, resp.Message, &replication.LedgerReplicationResponse{})
	msg := resp.Message.(*replication.LedgerReplicationResponse)
	require.Equal(t, replication.BlocksNotFound, msg.Status)
	require.Contains(t, msg.Message, "Node does not have block: 11, latest block: 4")
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	// partition id is valid, but does not match
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 2,
		PartitionID:      0xFFFFFFFF,
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	resp = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationResp)
	require.NotNil(t, resp)
	require.IsType(t, resp.Message, &replication.LedgerReplicationResponse{})
	msg = resp.Message.(*replication.LedgerReplicationResponse)
	require.Equal(t, replication.WrongShard, msg.Status)
	require.Equal(t, msg.Message, "Wrong partition/shard: requested FFFFFFFF-, I'm 01010101-")
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	// cases where node does not even respond
	// partition id is nil
	req := &replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 2,
		PartitionID:      0,
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, invalid partition identifier")
	req = &replication.LedgerReplicationRequest{
		NodeID:           tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 5,
		EndBlockNumber:   3,
		PartitionID:      tp.nodeConf.GetPartitionID(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, invalid block request range from 5 to 3")
	// unknown node identifier
	req = &replication.LedgerReplicationRequest{
		NodeID:           "",
		BeginBlockNumber: 2,
		PartitionID:      tp.nodeConf.GetPartitionID(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, node identifier is missing")
}

func createSameEpochBlock(t *testing.T, tp *SingleNodePartition, txs *testtxsystem.CounterTxSystem, uc *types.UnicityCertificate, txrs ...*types.TransactionRecord) (*types.Block, *types.UnicityCertificate) {
	return createBlock(t, tp, txs, uc, uc.InputRecord.Epoch, txrs...)
}

func createNextEpochBlock(t *testing.T, tp *SingleNodePartition, txs *testtxsystem.CounterTxSystem, uc *types.UnicityCertificate, txrs ...*types.TransactionRecord) (*types.Block, *types.UnicityCertificate) {
	return createBlock(t, tp, txs, uc, uc.InputRecord.Epoch+1, txrs...)
}

func createBlock(t *testing.T, tp *SingleNodePartition, txs *testtxsystem.CounterTxSystem, uc *types.UnicityCertificate, epoch uint64, txrs ...*types.TransactionRecord) (*types.Block, *types.UnicityCertificate) {
	newRound := uc.InputRecord.RoundNumber + 1
	var transactions = make([]*types.TransactionRecord, len(txrs))
	// simulate new block's state
	require.NoError(t, txs.BeginBlock(newRound))

	for i, txr := range txrs {
		transactions[i] = txr
		tx, err := txr.GetTransactionOrderV1()
		require.NoError(t, err)
		_, err = txs.Execute(tx)
		require.NoError(t, err)
	}
	state, err := txs.EndBlock()
	require.NoError(t, err)
	// create new block
	ucBytes, err := (&types.UnicityCertificate{
		Version: 1,
		TRHash:  make([]byte, 32),
		InputRecord: &types.InputRecord{
			Version:      1,
			Epoch:        epoch,
			RoundNumber:  newRound,
			Hash:         state.Root(),
			PreviousHash: uc.InputRecord.Hash,
			SummaryValue: state.Summary(),
			Timestamp:    uc.UnicitySeal.Timestamp,
		},
	}).MarshalCBOR()
	require.NoError(t, err)
	newBlock := &types.Block{
		Header: &types.Header{
			Version:           1,
			PartitionID:       uc.UnicityTreeCertificate.Partition,
			ShardID:           tp.nodeConf.shardConf.ShardID,
			ProposerID:        "test",
			PreviousBlockHash: uc.InputRecord.BlockHash,
		},
		Transactions:       transactions,
		UnicityCertificate: ucBytes,
	}
	_, err = newBlock.CalculateBlockHash(gocrypto.SHA256)
	require.NoError(t, err)
	newUC := &types.UnicityCertificate{Version: 1}
	require.NoError(t, newUC.UnmarshalCBOR(newBlock.UnicityCertificate))
	require.NoError(t, txs.Commit(newUC))

	newUC, _, err = tp.CreateUnicityCertificate(
		newUC.InputRecord,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	newBlock.UnicityCertificate, err = copyUC(t, newUC).MarshalCBOR()
	require.NoError(t, err)
	return newBlock, newUC
}

// newer UC is received, putting the node into recovery
// ledger replication request is sent
// ledger replication request is received with invalid UC.IR.SumOfEarnedFees => recovery fails
func TestNode_HandleLedgerReplicationResponse_SumOfEarnedFeesMismatch(t *testing.T) {
	tp := runSingleValidatorNodePartition(t, &testtxsystem.CounterTxSystem{Fee: 1})
	tp.WaitHandshake(t)
	uc0 := tp.GetCommittedUC(t)

	// create a block with single tx with fee=1 but sumOfEarnedFees=0
	system := &testtxsystem.CounterTxSystem{}
	newBlock1, uc1 := createSameEpochBlock(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(t, uc1)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)

	// when the replication response is received
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status:           replication.Ok,
		Blocks:           []*types.Block{newBlock1},
		FirstBlockNumber: uc1.GetRoundNumber(),
		LastBlockNumber:  uc1.GetRoundNumber(),
	})

	// then recovery should fail
	testevent.ContainsEvent(t, tp.eh, event.StateReverted)

	// and new replication request is sent
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)

	require.Equal(t, recovering, tp.partition.status.Load())
}

func copyUC(t *testing.T, uc *types.UnicityCertificate) *types.UnicityCertificate {
	bytes, err := types.Cbor.Marshal(uc)
	require.NoError(t, err)
	newUC := &types.UnicityCertificate{Version: 1}
	require.NoError(t, types.Cbor.Unmarshal(bytes, newUC))
	return newUC
}

func copyBlock(t *testing.T, b *types.Block) *types.Block {
	bytes, err := types.Cbor.Marshal(b)
	require.NoError(t, err)
	newBlock := &types.Block{}
	require.NoError(t, types.Cbor.Unmarshal(bytes, newBlock))
	return newBlock
}
