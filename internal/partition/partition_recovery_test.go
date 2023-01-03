package partition

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/partition/event"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/internal/txsystem"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	bl := tp.GetLatestBlock()
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
}

func TestNode_RecoverBlocks(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	genesisBlock := tp.GetLatestBlock()

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
