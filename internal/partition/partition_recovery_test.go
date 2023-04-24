package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testnetwork "github.com/alphabill-org/alphabill/internal/testutils/network"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNode_LedgerReplicationRequestTimeout(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	bl := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &certificates.InputRecord{
		PreviousHash: bl.UnicityCertificate.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: bl.UnicityCertificate.InputRecord.SummaryValue,
		RoundNumber:  bl.UnicityCertificate.InputRecord.RoundNumber + 1,
	}
	uc, err := tp.CreateUnicityCertificate(
		ir,
		bl.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// on timeout second request is sent
	require.Eventually(t, RequestReceived(tp, network.ProtocolLedgerReplicationReq), ledgerReplicationTimeout+time.Second, test.WaitTick)
	require.Equal(t, 1, len(reqs))
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_differentIR(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	bl := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// simulate UC with different state hash and block hash
	ir := &certificates.InputRecord{
		PreviousHash: bl.UnicityCertificate.InputRecord.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: bl.UnicityCertificate.InputRecord.SummaryValue,
		RoundNumber:  bl.UnicityCertificate.InputRecord.RoundNumber + 1,
	}
	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		bl.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(repeatUC)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	// increment round number
	irNew := &certificates.InputRecord{
		PreviousHash: ir.Hash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: bl.UnicityCertificate.InputRecord.SummaryValue,
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
	tp.SubmitUC(t, uc)

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("partition node didn't shut down within timeout")
	}
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withNoProposal(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := RunSingleNodePartition(t, system)
	bl := tp.GetLatestBlock(t)

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// send new UC
	rootRound := bl.UnicityCertificate.UnicitySeal.RootChainRoundNumber
	partitionRound := bl.UnicityCertificate.InputRecord.RoundNumber
	sum := bl.UnicityCertificate.InputRecord.SummaryValue
	rootRound++
	partitionRound++
	newStateHash := test.RandomBytes(32)
	ir := &certificates.InputRecord{
		PreviousHash: bl.UnicityCertificate.InputRecord.Hash,
		Hash:         newStateHash,
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
	}
	uc, err := tp.CreateUnicityCertificate(
		ir,
		rootRound,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(uc)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	rootRound++
	partitionRound++
	ir = &certificates.InputRecord{
		PreviousHash: newStateHash,
		Hash:         test.RandomBytes(32),
		BlockHash:    test.RandomBytes(32),
		SummaryValue: sum,
		RoundNumber:  partitionRound,
	}
	uc, err = tp.CreateUnicityCertificate(
		ir,
		rootRound,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(uc)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status.Load())
}

func TestNode_RecoverBlocks(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send back the response with 2 blocks
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock1, newBlock2},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3},
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// test get interfaces
	nr, err := tp.partition.GetLatestRoundNumber()
	require.NoError(t, err)
	require.Equal(t, uint64(4), nr)
	latestBlock, err := tp.partition.GetLatestBlock()
	require.NoError(t, err)
	require.True(t, proto.Equal(latestBlock, newBlock3))
	b, err := tp.partition.GetBlock(context.Background(), 0)
	require.ErrorContains(t, err, "block number 0 does not exist")
	require.Nil(t, b)
	b, err = tp.partition.GetBlock(context.Background(), 1)
	require.NoError(t, err)
	require.True(t, proto.Equal(b, genesisBlock))
	b, err = tp.partition.GetBlock(context.Background(), 2)
	require.NoError(t, err)
	require.True(t, proto.Equal(b, newBlock1))
	b, err = tp.partition.GetBlock(context.Background(), 3)
	require.NoError(t, err)
	require.True(t, proto.Equal(b, newBlock2))
	b, err = tp.partition.GetBlock(context.Background(), 4)
	require.NoError(t, err)
	require.True(t, proto.Equal(b, newBlock3))
	require.True(t, proto.Equal(b, latestBlock))
	// on not found nil is returned
	b, err = tp.partition.GetBlock(context.Background(), 5)
	require.NoError(t, err)
	require.Nil(t, b)
	require.Equal(t, []byte{1, 1, 1, 1}, tp.partition.SystemIdentifier())
}

func TestNode_RecoverSkipsRequiredBlock(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// skip block 2 and send block 3 only
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3},
		},
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// still recovering
	require.Equal(t, recovering, tp.partition.status.Load())
	// node is asking for missing block 2
	reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	msg := reqs[0].Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, msg.BeginBlockNumber, uint64(2))
}

func TestNode_RecoverSkipsBlocksAndSendMixedBlocks(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send back the response with 2 blocks
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, newBlock2},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, newBlock3},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the response with last block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock4},
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.RecoveryFinished)
	require.Equal(t, normal, tp.partition.status.Load())
	// and now out of the blue a response with blocks 2,3 is received again
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, newBlock3},
		},
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
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	altBlock3 := proto.Clone(newBlock3).(*block.Block)
	altBlock3.Transactions = append(altBlock3.Transactions, moneytesttx.RandomBillTransfer(t))
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, altBlock3},
		},
	})
	// make sure replication request is sent again and that block 3 is asked again
	require.Eventually(t, func() bool { return len(tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)) == 1 }, 1*time.Second, 10*time.Millisecond)
	reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	req := reqs[0].Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, uint64(3), req.BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3, newBlock4},
		},
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
	t.Cleanup(cancel)
	StartSingleNodePartition(ctx, t, tp)

	genesisBlock := tp.GetLatestBlock(t)
	system := &testtxsystem.CounterTxSystem{}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	altBlock3 := proto.Clone(newBlock3).(*block.Block)
	altBlock3.NodeIdentifier = ""
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send back the response with 2 blocks
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, altBlock3},
		},
	})
	// make sure replication request is sent again and that block 3 is asked again
	require.Eventually(t, func() bool { return len(tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)) == 1 }, 1*time.Second, 10*time.Millisecond)
	reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	req := reqs[0].Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, uint64(3), req.BeginBlockNumber)
	require.Equal(t, recovering, tp.partition.status.Load())

	// send back the block 2 again, but also block 3
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3, newBlock4},
		},
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
	genesisBlock := &block.Block{
		SystemIdentifier:   tp.nodeDeps.genesis.SystemDescriptionRecord.SystemIdentifier,
		NodeIdentifier:     "genesis",
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: tp.nodeDeps.genesis.GetCertificate(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	StartSingleNodePartition(ctx, t, tp)

	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send all missing blocks
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2},
		},
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// still recovering
	require.Equal(t, recovering, tp.partition.status.Load())
	// expect a ledger replication request
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send blocks 3, 4, but set error first
	db.MockWriteError(fmt.Errorf("disk is full"))
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3, newBlock4},
		},
	})
	// wait for message to be processed
	require.Eventually(t, func() bool { return len(tp.mockNet.MessageCh) == 0 }, 1*time.Second, 10*time.Millisecond)
	// db failed to persist block 3 because disk is full, block 3 is asked again in a loop
	// expect a ledger replication request
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	msg := reqs[0].Message.(*replication.LedgerReplicationRequest)
	require.Equal(t, uint64(3), msg.BeginBlockNumber)
	// clear error and make sure node still recovers
	db.MockWriteError(nil)
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationReq)
	// send all missing blocks 3, 4 and make sure that node now recovers
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock3, newBlock4},
		},
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
	genesisBlock := &block.Block{
		SystemIdentifier:   tp.nodeDeps.genesis.SystemDescriptionRecord.SystemIdentifier,
		NodeIdentifier:     "genesis",
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: tp.nodeDeps.genesis.GetCertificate(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	StartSingleNodePartition(ctx, t, tp)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(genesisBlock.UnicityCertificate)
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	require.Len(t, newBlock2.Transactions, 1)
	// submit transaction
	require.NoError(t, tp.SubmitTx(newBlock2.Transactions[0]))
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
	tp.partition.handleT1TimeoutEvent()
	// block proposal is sent
	require.Eventually(t, func() bool {
		return len(tp.mockNet.SentMessages(network.ProtocolBlockCertification)) == 1
	}, test.WaitDuration, test.WaitTick, "block certification request not found")
	// set DB in error state
	db.MockWriteError(fmt.Errorf("disk is full"))
	// submit UC status from root
	tp.SubmitUnicityCertificate(newBlock2.UnicityCertificate)
	// block is requested from other nodes
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// expect store fails and node enters recovery
	require.Equal(t, recovering, tp.partition.status.Load())
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	msg := reqs[0].Message.(*replication.LedgerReplicationRequest)
	require.NotNil(t, msg)
	// make sure block 2 is asked
	require.Equal(t, uint64(2), msg.BeginBlockNumber)
	// reset error
	db.MockWriteError(nil)
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2},
		},
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
	genesisBlock := &block.Block{
		SystemIdentifier:   tp.nodeDeps.genesis.SystemDescriptionRecord.SystemIdentifier,
		NodeIdentifier:     "genesis",
		Transactions:       []*txsystem.Transaction{},
		UnicityCertificate: tp.nodeDeps.genesis.GetCertificate(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	StartSingleNodePartition(ctx, t, tp)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(genesisBlock.UnicityCertificate)
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	require.Len(t, newBlock2.Transactions, 1)
	// mock error situation, every next write will fail with error
	db.MockWriteError(fmt.Errorf("disk full"))
	require.NoError(t, tp.SubmitTx(newBlock2.Transactions[0]))
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
	tp.partition.handleT1TimeoutEvent()
	// no block certification request is sent
	require.Len(t, tp.mockNet.SentMessages(network.ProtocolBlockCertification), 0)
	// make no certification request is sent, proposal is not stored
	var pr block.PendingBlockProposal
	found, err := db.Read(util.Uint32ToBytes(proposalKey), &pr)
	require.NoError(t, err)
	require.False(t, found)
	tp.SubmitUnicityCertificate(newBlock2.UnicityCertificate)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	// block is requested from other nodes
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	// reset error
	db.MockWriteError(nil)
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2},
		},
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
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock2 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)
	newBlock4 := createNewBlockOutsideNode(t, tp, system, newBlock3)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock4.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status.Load())
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	var reqs []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		reqs = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
		return len(reqs) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send back the response with nil block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{nil},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock := proto.Clone(newBlock2).(*block.Block)
	illegalBlock.SystemIdentifier = []byte{0xFF, 0xFF, 0xFF, 0xFF}
	// send back the response with nil block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{illegalBlock},
		},
	})
	illegalBlock = proto.Clone(newBlock2).(*block.Block)
	illegalBlock.SystemIdentifier = nil
	// send back the response with nil block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{illegalBlock},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	illegalBlock = proto.Clone(newBlock2).(*block.Block)
	illegalBlock.UnicityCertificate = nil
	// send back the response with nil block
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{illegalBlock},
		},
	})
	require.Equal(t, recovering, tp.partition.status.Load())
	// send all blocks and assume full recovery
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{newBlock2, newBlock3, newBlock4},
		},
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
	genesisBlockNumber := tp.GetLatestBlock(t).UnicityCertificate.InputRecord.RoundNumber

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// generate 4 blocks with 3 tx each (but only 2 blocks will be matched and sent)
	for i := 0; i < 4; i++ {
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
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
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
			BeginBlockNumber: genesisBlockNumber + 1,
			SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
		},
	})

	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)

	//make sure response is sent
	var resp []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		resp = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
		return len(resp) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.LedgerReplicationResponse_OK, resp[0].Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, tp.nodeDeps.peer.ID().String(), resp[0].ID.String())
	require.Equal(t, 2, len(resp[0].Message.(*replication.LedgerReplicationResponse).Blocks))

	tp.eh.Reset()
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	tp.partition.configuration.replicationConfig.maxBlocks = 1
	//send replication request, it will hit block replication limit
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
			BeginBlockNumber: genesisBlockNumber + 1,
			SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	require.Eventually(t, func() bool {
		resp = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
		return len(resp) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.LedgerReplicationResponse_OK, resp[0].Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, tp.nodeDeps.peer.ID().String(), resp[0].ID.String())
	require.Equal(t, 1, len(resp[0].Message.(*replication.LedgerReplicationResponse).Blocks))
}

func TestNode_RespondToInvalidReplicationRequest(t *testing.T) {
	tp := RunSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 5))
	genesisBlockNumber := tp.GetLatestBlock(t).UnicityCertificate.InputRecord.RoundNumber

	tp.partition.startNewRound(context.Background(), tp.partition.luc.Load())

	// generate 4 blocks with 3 tx each (but only 2 blocks will be matched and sent)
	for i := 0; i < 4; i++ {
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
		require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
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
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
			BeginBlockNumber: 11,
			SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	//make sure response is sent
	var resp []testnetwork.PeerMessage
	require.Eventually(t, func() bool {
		resp = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
		return len(resp) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	msg := resp[0].Message.(*replication.LedgerReplicationResponse)
	require.Equal(t, replication.LedgerReplicationResponse_BLOCKS_NOT_FOUND, msg.Status)
	require.Contains(t, msg.Message, "Node does not have block: 11, latest block: 5")
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	// system id is valid, but does not match
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
			BeginBlockNumber: 2,
			SystemIdentifier: []byte{0xFF, 0xFF, 0xFF, 0xFF},
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	require.Eventually(t, func() bool {
		resp = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
		return len(resp) != 0
	}, test.WaitDuration, test.WaitTick)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	msg = resp[0].Message.(*replication.LedgerReplicationResponse)
	require.Equal(t, replication.LedgerReplicationResponse_UNKNOWN_SYSTEM_IDENTIFIER, msg.Status)
	require.Contains(t, msg.Message, "Unknown system identifier: FFFFFFFF")
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	// cases where node does not even respond
	// system id is nil
	req := &replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
		BeginBlockNumber: 2,
		SystemIdentifier: nil,
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(req), "invalid request, invalid system identifier")
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   tp.nodeDeps.peer.ID().String(),
		BeginBlockNumber: 5,
		EndBlockNumber:   3,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(req), "invalid request, invalid block request range from 5 to 3")
	// unknown node identifier
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   "",
		BeginBlockNumber: 2,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(req), "invalid request, node identifier is missing")
	// no node identifier, cannot respond
	req = &replication.LedgerReplicationRequest{
		NodeIdentifier:   "foo",
		BeginBlockNumber: 2,
		SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
	}
	require.ErrorContains(t, tp.partition.handleLedgerReplicationRequest(req), "unknown node, signing public key for id foo not found")
}

func createNewBlockOutsideNode(t *testing.T, tp *SingleNodePartition, system *testtxsystem.CounterTxSystem, currentBlock *block.Block) *block.Block {
	// simulate new block's state
	system.BeginBlock(currentBlock.UnicityCertificate.InputRecord.RoundNumber + 1)
	_ = system.Execute(nil)
	state, _ := system.EndBlock()
	system.Commit()

	// create new block
	newBlock := proto.Clone(currentBlock).(*block.Block)
	newBlock.UnicityCertificate.InputRecord.RoundNumber = currentBlock.UnicityCertificate.InputRecord.RoundNumber + 1
	newBlock.PreviousBlockHash, _ = currentBlock.Hash(system, gocrypto.SHA256)
	newBlock.Transactions = make([]*txsystem.Transaction, 1)
	newBlock.Transactions[0] = moneytesttx.RandomBillTransfer(t)

	// send UC certifying new block
	ir := newBlock.UnicityCertificate.InputRecord
	ir.PreviousHash = ir.Hash
	ir.BlockHash, _ = newBlock.Hash(system, gocrypto.SHA256)
	ir.Hash = state.Root()
	ir.SummaryValue = state.Summary()

	newUC, err := tp.CreateUnicityCertificate(
		ir,
		currentBlock.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	newBlock.UnicityCertificate = newUC
	return newBlock
}
