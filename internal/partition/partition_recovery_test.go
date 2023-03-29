package partition

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_differentIR(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	bl := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// prepare proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)
	// send UC with different IR hash
	ir := proto.Clone(bl.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		bl.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		bl.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(repeatUC)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	reqs := tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	newerUC, err := tp.CreateUnicityCertificate(
		ir,
		repeatUC.UnicitySeal.RootChainRoundNumber+1,
		repeatUC.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(newerUC)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status)
}

// AB-714 If before shutting down nodes managed to send certification requests,
// the proposal is valid and must be restored correctly since the latest UC will certify it
// that is, node does not need to send replication request, but instead should restore proposal and accept UC to finalize the block
func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withPendingProposal_sameIR(t *testing.T) {
	t.SkipNow() // TODO fix test

	store := store.NewInMemoryBlockStore()
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithBlockStore(store))
	t.Cleanup(func() {
		tp.Close()
	})

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
	tp.Restart(t)

	// block finalization
	tp.SubmitUC(t, uc)
}

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery_withNoProposal(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	bl := tp.GetLatestBlock(t)

	err := tp.partition.startNewRound(tp.partition.luc)
	require.NoError(t, err)

	// send UC with different block hash
	ir := proto.Clone(bl.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.BlockHash = test.RandomBytes(32)

	repeatUC, err := tp.CreateUnicityCertificate(
		ir,
		bl.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		bl.UnicityCertificate.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(repeatUC)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	require.Equal(t, recovering, tp.partition.status)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)
	// make sure replication request is sent
	reqs := tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})

	// send newer UC and check LUC is updated and node still recovering
	tp.eh.Reset()
	newerUC, err := tp.CreateUnicityCertificate(
		ir,
		repeatUC.UnicitySeal.RootChainRoundNumber+1,
		repeatUC.UnicitySeal.PreviousHash,
	)
	require.NoError(t, err)
	tp.SubmitUnicityCertificate(newerUC)
	testevent.ContainsEvent(t, tp.eh, event.LatestUnicityCertificateUpdated)
	require.Equal(t, recovering, tp.partition.status)
}

func TestNode_RecoverBlocks(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	genesisBlock := tp.GetLatestBlock(t)

	system := &testtxsystem.CounterTxSystem{}
	newBlock1 := createNewBlockOutsideNode(t, tp, system, genesisBlock)
	newBlock2 := createNewBlockOutsideNode(t, tp, system, newBlock1)
	newBlock3 := createNewBlockOutsideNode(t, tp, system, newBlock2)

	// prepare proposal, send "newer" UC, revert state and start recovery
	tp.SubmitT1Timeout(t)
	tp.SubmitUnicityCertificate(newBlock3.UnicityCertificate)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, recovering, tp.partition.status)
	testevent.ContainsEvent(t, tp.eh, event.RecoveryStarted)

	// make sure replication request is sent
	reqs := tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
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
	require.Equal(t, recovering, tp.partition.status)

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
	require.Equal(t, idle, tp.partition.status)
}

func TestNode_RespondToReplicationRequest(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{}, WithReplicationParams(3, 5))
	defer tp.Close()
	genesisBlockNumber := tp.GetLatestBlock(t).UnicityCertificate.InputRecord.RoundNumber

	err := tp.partition.startNewRound(tp.partition.luc)
	require.NoError(t, err)

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
	peer := "16Uiu2HAm826WzV3ZDwtEA93VJVxMPSvyrVUK7ArifTmhr3CwLzMj"
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   peer,
			BeginBlockNumber: genesisBlockNumber + 1,
			SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
		},
	})

	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)

	//make sure response is sent
	resp := tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.LedgerReplicationResponse_OK, resp[0].Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, peer, resp[0].ID.String())
	require.Equal(t, 2, len(resp[0].Message.(*replication.LedgerReplicationResponse).Blocks))

	tp.eh.Reset()
	tp.mockNet.ResetSentMessages(network.ProtocolLedgerReplicationResp)
	tp.partition.configuration.replicationConfig.maxBlocks = 1
	//send replication request, it will hit block replication limit
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     "from-test",
		Protocol: network.ProtocolLedgerReplicationReq,
		Message: &replication.LedgerReplicationRequest{
			NodeIdentifier:   peer,
			BeginBlockNumber: genesisBlockNumber + 1,
			SystemIdentifier: tp.nodeConf.GetSystemIdentifier(),
		},
	})
	testevent.ContainsEvent(t, tp.eh, event.ReplicationResponseSent)
	resp = tp.mockNet.SentMessages(network.ProtocolLedgerReplicationResp)
	require.Equal(t, 1, len(resp))
	require.IsType(t, resp[0].Message, &replication.LedgerReplicationResponse{})
	require.Equal(t, replication.LedgerReplicationResponse_OK, resp[0].Message.(*replication.LedgerReplicationResponse).Status)
	require.Equal(t, peer, resp[0].ID.String())
	require.Equal(t, 1, len(resp[0].Message.(*replication.LedgerReplicationResponse).Blocks))
}

func createNewBlockOutsideNode(t *testing.T, tp *SingleNodePartition, system *testtxsystem.CounterTxSystem, currentBlock *block.Block) *block.Block {
	// simulate new block's state
	_ = system.Execute(nil)
	state, _ := system.EndBlock()
	system.Commit()

	// create new block
	newBlock := proto.Clone(currentBlock).(*block.Block)
	newBlock.UnicityCertificate.InputRecord.RoundNumber = currentBlock.UnicityCertificate.InputRecord.RoundNumber + 1
	newBlock.PreviousBlockHash, _ = currentBlock.Hash(system, tp.partition.configuration.hashAlgorithm)
	newBlock.Transactions = make([]*txsystem.Transaction, 1)
	newBlock.Transactions[0] = moneytesttx.RandomBillTransfer(t)

	// send UC certifying new block
	ir := newBlock.UnicityCertificate.InputRecord
	ir.PreviousHash = ir.Hash
	ir.BlockHash, _ = newBlock.Hash(system, tp.partition.configuration.hashAlgorithm)
	ir.Hash = state.Root()
	ir.SummaryValue = state.Summary()

	newUC, err := tp.CreateUnicityCertificate(
		ir,
		currentBlock.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
		currentBlock.UnicityCertificate.UnicitySeal.Hash,
	)
	require.NoError(t, err)
	newBlock.UnicityCertificate = newUC
	return newBlock
}
