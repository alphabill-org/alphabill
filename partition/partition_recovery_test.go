package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/replication"
	"github.com/alphabill-org/alphabill/partition/event"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestNode_LedgerReplicationRequestTimeout(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	uc1 := tp.GetCommittedUC(t)
	order := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(order))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &types.InputRecord{
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  uc1.InputRecord.RoundNumber + 1,
	}
	uc2, err := tp.CreateUnicityCertificate(
		ir,
		uc1.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc2)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	// on timeout second request is sent
	require.Eventually(t, RequestReceived(tp, network.ProtocolLedgerReplicationReq), ledgerReplicationTimeout+time.Second, test.WaitTick)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_differentIR(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	uc1 := tp.GetCommittedUC(t)
	order := testtransaction.NewTransactionOrder(t)
	require.NoError(t, tp.SubmitTx(order))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &types.InputRecord{
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  uc1.InputRecord.RoundNumber + 1,
	}
	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		uc1.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(repeatUC)

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
		PreviousHash: ir.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: uc1.InputRecord.SummaryValue,
		RoundNumber:  ir.RoundNumber + 1,
	}
	newerUC, err := tp.CreateUnicityCertificate(
		irNew,
		repeatUC.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(newerUC)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())
}

// AB-714 If before shutting down nodes managed to send certification requests,
// the proposal is valid and must be restored correctly since the latest UC will certify it
// that is, node does not need to send replication request, but instead should restore proposal and accept UC to finalize the block
func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_sameIR(t *testing.T) {
	store := memorydb.New()
	ctx, cancel := context.WithCancel(context.Background())
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)

	// create new block
	tp.CreateBlock(t)

	// create new proposal and certify it (but not yet finalize the block on the partition side)
	tp.SubmitT1Timeout(t)
	uc := tp.IssueBlockUC(t)

	system := &testtxsystem.CounterTxSystem{}
	system.Commit(tp.nodeConf.genesis.Certificate)
	tp.nodeDeps.txSystem = system

	tp.nodeDeps.nodeOptions = append(tp.nodeDeps.nodeOptions, WithLeaderSelector(&TestLeaderSelector{
		leader:      "",
		currentNode: "1",
	}))

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
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.BlockFinalized)

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("partition node didn't shut down within timeout")
	}
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store := memorydb.New()
	ctx, cancel := context.WithCancel(context.Background())
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())
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
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// create new proposal and certify it (but not yet finalize the block on the partition side)
	tp.SubmitT1Timeout(t)
	uc := tp.IssueBlockUC(t)
	bl := tp.GetLatestBlock(t)
	latestRound := bl.GetRoundNumber()

	// now assume while the node was offline, other validators produced several new blocks, all empty
	// that is, round number has been incremented, but the state hash is the same
	uc.InputRecord.RoundNumber += 5
	uc, err := tp.CreateUnicityCertificate(
		uc.InputRecord,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	// submit UC, node must recover the gap of 5 blocks
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	require.Equal(t, recovering, tp.partition.status.Load())
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, latestRound+1, msg.BeginBlockNumber)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_noPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store := memorydb.New()
	ctx, cancel := context.WithCancel(context.Background())
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// create new block
	tp.CreateBlock(t)
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	bl := tp.GetLatestBlock(t)
	latestRound := bl.GetRoundNumber()
	uc := bl.UnicityCertificate

	// now assume while the node was offline, other validators produced several new blocks, all empty
	// that is, round number has been incremented, but the state hash is the same
	uc.InputRecord.RoundNumber += 5
	uc, err := tp.CreateUnicityCertificate(
		uc.InputRecord,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	// submit UC, node must recover the gap of 5 blocks
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	require.Equal(t, recovering, tp.partition.status.Load())
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	msg := req.Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, latestRound+1, msg.BeginBlockNumber)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_missedPendingProposal_sameIR_butDifferentBlocks(t *testing.T) {
	store := memorydb.New()
	ctx, cancel := context.WithCancel(context.Background())
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	done := StartSingleNodePartition(ctx, t, tp)
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("partition node didn't shut down within timeout")
		}
	})
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// create new block
	tp.CreateBlock(t)
	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	bl := tp.GetLatestBlock(t)
	latestRound := bl.GetRoundNumber()
	uc := bl.UnicityCertificate

	// now assume the node missed the proposal due to a network hiccup, other validators finalized _one_ empty block
	// that is, round number has been incremented, but the state hash is the same
	// since there's no proposal, the node will start a new round (the one that has been already finalized)
	ir := uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 1
	uc, err := tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.NewRoundStarted)
	// okay, now let's finalize another round, node should start the recovery
	ir = uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 1
	uc, err = tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	require.Equal(t, recovering, tp.partition.status.Load())
	// let's submit another UC and make sure node updates the LUC and keeps recovering
	ir = uc.InputRecord.NewRepeatIR()
	ir.RoundNumber += 1
	uc, err = tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)
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
	tp := RunSingleNodePartition(t, system)
	uc1 := tp.GetCommittedUC(t)

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// send new UC
	rootRound := uc1.UnicitySeal.RootChainRoundNumber
	partitionRound := uc1.InputRecord.RoundNumber
	sum := uc1.InputRecord.SummaryValue
	rootRound++
	partitionRound++
	newStateHash := test.RandomBytes(32)
	ir := &types.InputRecord{
		PreviousHash: uc1.InputRecord.Hash,
		Hash:         newStateHash,
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
	}
	uc2, err := tp.CreateUnicityCertificate(
		ir,
		rootRound,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(uc2)

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
		PreviousHash: newStateHash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
	}
	uc3, err := tp.CreateUnicityCertificate(
		ir,
		rootRound,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc3)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())
}

func TestNode_RecoverBlocks(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock2},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock3},
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	nr, err := tp.partition.GetLatestRoundNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(3), nr)
	latestBlock := tp.GetLatestBlock(t)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(latestBlock, newBlock3))
	b, err := tp.partition.GetBlock(context.Background(), 0)
	require.ErrorContains(t, err, "node does not have block: 0, first block: 1")
	require.Nil(t, b)
	b, err = tp.partition.GetBlock(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock1))
	b, err = tp.partition.GetBlock(context.Background(), 2)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock2))
	b, err = tp.partition.GetBlock(context.Background(), 3)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock3))
	require.True(t, reflect.DeepEqual(b, latestBlock))
	// on not found nil is returned
	b, err = tp.partition.GetBlock(context.Background(), 4)
	require.NoError(t, err)
	require.Nil(t, b)
	require.Equal(t, []byte{1, 1, 1, 1}, tp.partition.SystemIdentifier())
}

func TestNode_RecoverBlocks_withEmptyBlocksChangingState(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{EndBlockChangesState: true})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3empty := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate)
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3empty.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock5empty := createNewBlockOutsideNode(t, tp, system, newBlock4.UnicityCertificate)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock5empty.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock2},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock3empty, newBlock4, newBlock5empty},
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	nr, err := tp.partition.GetLatestRoundNumber(context.Background())
	require.NoError(t, err)
	require.Equal(t, uint64(5), nr)
	latestBlock := tp.GetLatestBlock(t)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(latestBlock, newBlock5empty))
	b, err := tp.partition.GetBlock(context.Background(), 0)
	require.ErrorContains(t, err, "node does not have block: 0, first block: 1")
	require.Nil(t, b)
	b, err = tp.partition.GetBlock(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock1))
	b, err = tp.partition.GetBlock(context.Background(), 2)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock2))
	b, err = tp.partition.GetBlock(context.Background(), 3)
	require.NoError(t, err)
	require.NotNil(t, b) // newBlock3empty
	b, err = tp.partition.GetBlock(context.Background(), 4)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, newBlock4))
	b, err = tp.partition.GetBlock(context.Background(), 5)
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(b, latestBlock))
	// on not found nil is returned
	b, err = tp.partition.GetBlock(context.Background(), 6)
	require.NoError(t, err)
	require.Nil(t, b)
	require.Equal(t, []byte{1, 1, 1, 1}, tp.partition.SystemIdentifier())
}

func TestNode_RecoverSkipsRequiredBlock(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{EndBlockChangesState: true})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0)
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// skip block 1 and send block 2 only
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock2},
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
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock3},
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
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock1},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 1 again, but also block 2
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock2},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock3},
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// and now out of the blue a response with blocks 1,2 is received again
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock2},
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverReceivesInvalidBlock(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	altBlock2 := copyBlock(t, newBlock2)
	altBlock2.Transactions = append(altBlock2.Transactions, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, altBlock2},
	})
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	require.Equal(t, uint64(2), req.Message.(*replication.LedgerReplicationRequest).BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock2, newBlock3},
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverReceivesInvalidBlockNoBlockProposerId(t *testing.T) {
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
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
	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	altBlock2 := copyBlock(t, newBlock2)
	altBlock2.Header.ProposerID = ""
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, altBlock2},
	})
	// make sure replication request is sent again and that block 3 is asked again
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	require.Equal(t, uint64(2), req.Message.(*replication.LedgerReplicationRequest).BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock2, newBlock3},
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
	db := memorydb.New()
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{}
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(db))
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
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	// send all missing blocks
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1},
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
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock2, newBlock3},
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
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock2, newBlock3},
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
	db := memorydb.New()
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{}
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(db))
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
	tp.SubmitUnicityCertificate(uc0)
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	require.Len(t, newBlock1.Transactions, 1)
	// submit transaction
	require.NoError(t, tp.SubmitTx(newBlock1.Transactions[0].TransactionOrder))
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
	tp.SubmitUnicityCertificate(newBlock1.UnicityCertificate)
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
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1},
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
	db := memorydb.New()
	// used to generate test blocks
	system := &testtxsystem.CounterTxSystem{}
	tp := SetupNewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(db))
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
	tp.SubmitUnicityCertificate(uc0)
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	require.Len(t, newBlock1.Transactions, 1)
	// mock error situation, every next write will fail with error
	db.MockWriteError(fmt.Errorf("disk full"))
	require.NoError(t, tp.SubmitTx(newBlock1.Transactions[0].TransactionOrder))
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
	var pr pendingBlockProposal
	found, err := db.Read(util.Uint32ToBytes(proposalKey), &pr)
	require.NoError(t, err)
	require.False(t, found)
	tp.SubmitUnicityCertificate(newBlock1.UnicityCertificate)
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
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1},
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RecoverySendInvalidLedgerReplicationReplies(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	uc0 := tp.GetCommittedUC(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1.UnicityCertificate, testtransaction.NewTransactionRecord(t))
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2.UnicityCertificate, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)
	require.IsType(t, req.Message, &replication.LedgerReplicationRequest{})
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{nil},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock := copyBlock(t, newBlock1)
	illegalBlock.Header.SystemID = []byte{0xFF, 0xFF, 0xFF, 0xFF}
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{illegalBlock},
	})
	illegalBlock = copyBlock(t, newBlock1)
	illegalBlock.Header.SystemID = nil
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{illegalBlock},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock = copyBlock(t, newBlock1)
	illegalBlock.UnicityCertificate = nil
	// send back the response with nil block
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{illegalBlock},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	// send all blocks and assume full recovery
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1, newBlock2, newBlock3},
	})
	// wait for message to be processed and expect recovery finished event
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	// and status is normal
	require.Eventually(t, func() bool {
		return tp.partition.status.Load() == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_RespondToReplicationRequest(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 5))
	genesisBlockNumber := tp.GetCommittedUC(t).GetRoundNumber()

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

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
	latestBlockNumber := tp.GetLatestBlock(t).UnicityCertificate.InputRecord.RoundNumber
	require.Equal(t, uint64(4), latestBlockNumber-genesisBlockNumber)

	//send replication request, it will hit tx replication limit
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: genesisBlockNumber + 1,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
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
	tp.partition.configuration.replicationConfig.maxBlocks = 1
	//send replication request, it will hit block replication limit
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: genesisBlockNumber + 1,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
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
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 5))
	genesisBlockNumber := tp.GetCommittedUC(t).GetRoundNumber()

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

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
	latestBlockNumber := tp.GetLatestBlock(t).UnicityCertificate.InputRecord.RoundNumber
	require.Equal(t, uint64(4), latestBlockNumber-genesisBlockNumber)
	// does not have the block 11
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 11,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
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
	// system id is valid, but does not match
	tp.mockNet.Receive(&replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 2,
		SystemIdentifier: []byte{0xFF, 0xFF, 0xFF, 0xFF},
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	resp = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationResp)
	require.NotNil(t, resp)
	require.IsType(t, resp.Message, &replication.LedgerReplicationResponse{})
	msg = resp.Message.(*replication.LedgerReplicationResponse)
	require.Equal(t, replication.UnknownSystemIdentifier, msg.Status)
	require.Contains(t, msg.Message, "Unknown system identifier: FFFFFFFF")
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	// cases where node does not even respond
	// system id is nil
	req := &replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 2,
		SystemIdentifier: nil,
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, invalid system identifier")
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peerConf.ID.String(),
		BeginBlockNumber: 5,
		EndBlockNumber:   3,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, invalid block request range from 5 to 3")
	// unknown node identifier
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   "",
		BeginBlockNumber: 2,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "invalid request, node identifier is missing")
	// no node identifier, cannot respond
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   "foo",
		BeginBlockNumber: 2,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(context.Background(), req), "unknown node, signing public key for id foo not found")
}

func createNewBlockOutsideNode(t *testing.T, tp *SingleNodePartition, txs *testtxsystem.CounterTxSystem, uc *types.UnicityCertificate, txrs ...*types.TransactionRecord) *types.Block {
	// simulate new block's state
	txs.BeginBlock(uc.InputRecord.RoundNumber + 1)

	// create new block
	newBlock := &types.Block{
		Header: &types.Header{
			SystemID: uc.UnicityTreeCertificate.SystemIdentifier,
			ProposerID: "test",
		},
		UnicityCertificate: copyUC(t, uc),
	}
	newBlock.UnicityCertificate.InputRecord.RoundNumber = uc.InputRecord.RoundNumber + 1
	newBlock.Header.PreviousBlockHash = uc.InputRecord.BlockHash
	newBlock.Transactions = make([]*types.TransactionRecord, 0)
	for _, txr := range txrs {
		newBlock.Transactions = append(newBlock.Transactions, txr)
		_, err := txs.Execute(txr.TransactionOrder)
		require.NoError(t, err)
	}
	state, err := txs.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txs.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  newBlock.UnicityCertificate.InputRecord.RoundNumber,
		Hash:         state.Root(),
		SummaryValue: state.Summary(),
	}}))

	// send UC certifying new block
	ir := newBlock.UnicityCertificate.InputRecord
	ir.PreviousHash = ir.Hash
	ir.BlockHash, _ = newBlock.Hash(gocrypto.SHA256)
	ir.Hash = state.Root()
	ir.SummaryValue = state.Summary()

	newUC, err := tp.CreateUnicityCertificate(
		ir,
		uc.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	newBlock.UnicityCertificate = newUC
	return newBlock
}

// newer UC is received, putting the node into recovery
// ledger replication request is sent
// ledger replication request is received with invalid UC.IR.SumOfEarnedFees => recovery fails
func TestNode_HandleLedgerReplicationResponse_SumOfEarnedFeesMismatch(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{Fee: 1})
	uc0 := tp.GetCommittedUC(t)

	// create a block with single tx with fee=1 but sumOfEarnedFees=0
	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, uc0, testtransaction.NewTransactionRecord(t))

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock1.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	req := WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)

	// when the replication response is received
	tp.mockNet.Receive(&replication.LedgerReplicationResponse{
		Status: replication.Ok,
		Blocks: []*types.Block{newBlock1},
	})

	// then recovery should fail
	testevent.ContainsEvent(t, tp.eh, event.StateReverted)

	// and new replication request is sent
	req = WaitNodeRequestReceived(t, tp, network.ProtocolLedgerReplicationReq)
	require.NotNil(t, req)

	require.Equal(t, recovering, tp.partition.status.Load())
}

func copyUC(t *testing.T, uc *types.UnicityCertificate) *types.UnicityCertificate {
	bytes, err := cbor.Marshal(uc)
	require.NoError(t, err)
	newUC := &types.UnicityCertificate{}
	require.NoError(t, cbor.Unmarshal(bytes, newUC))
	return newUC
}

func copyBlock(t *testing.T, b *types.Block) *types.Block {
	bytes, err := cbor.Marshal(b)
	require.NoError(t, err)
	newBlock := &types.Block{}
	require.NoError(t, cbor.Unmarshal(bytes, newBlock))
	return newBlock
}
