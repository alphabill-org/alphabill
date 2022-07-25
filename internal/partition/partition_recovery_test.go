package partition

import (
	"encoding/json"
	"testing"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/txsystem"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestNode_HandleUnicityCertificate_RevertAndStartRecovery(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	bl := tp.GetLatestBlock()
	transfer := testtransaction.RandomBillTransfer()
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
	ContainsEvent(t, tp, EventTypeRecoveryStarted)
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
	ContainsEvent(t, tp, EventTypeRecoveryStarted)

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
	ContainsEvent(t, tp, EventTypeRecoveryFinished)
	require.Equal(t, idle, tp.partition.status)
}

func createNewBlockOutsideNode(t *testing.T, tp *SingleNodePartition, system *testtxsystem.CounterTxSystem, currentBlock *block.Block) *block.Block {
	// simulate new block's state
	_ = system.Execute(nil)
	state, _ := system.EndBlock()
	system.Commit()

	// create new block
	newBlock := proto.Clone(currentBlock).(*block.Block)
	newBlock.BlockNumber = currentBlock.BlockNumber + 1
	newBlock.PreviousBlockHash, _ = currentBlock.Hash(tp.partition.configuration.hashAlgorithm)
	newBlock.Transactions = make([]*txsystem.Transaction, 1)
	newBlock.Transactions[0] = testtransaction.RandomBillTransfer()

	// send UC certifying new block
	ir := newBlock.UnicityCertificate.InputRecord
	ir.PreviousHash = ir.Hash
	ir.BlockHash, _ = newBlock.Hash(tp.partition.configuration.hashAlgorithm)
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

func TestNode_CreateBlocks_andRecord(t *testing.T) {
	t.SkipNow()
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	b0 := tp.GetLatestBlock()
	b1, _ := createAndValidateBlock(t, tp)
	b2, _ := createAndValidateBlock(t, tp)
	b3, _ := createAndValidateBlock(t, tp)

	require.Greater(t, b3.GetBlockNumber(), b2.GetBlockNumber())
	require.Greater(t, b2.GetBlockNumber(), b1.GetBlockNumber())
	require.Greater(t, b1.GetBlockNumber(), b0.GetBlockNumber())

	util.WriteDebugJsonLog(logger, "Block 0 from test", b0)
	util.WriteDebugJsonLog(logger, "Block 1 from test", b1)
	util.WriteDebugJsonLog(logger, "Block 2 from test", b2)
	util.WriteDebugJsonLog(logger, "Block 3 from test", b3)
}

func createAndValidateBlock(t *testing.T, tp *SingleNodePartition) (*block.Block, *txsystem.Transaction) {
	tx := testtransaction.RandomBillTransfer()
	require.NoError(t, tp.SubmitTx(tx))
	ContainsEvent(t, tp, EventTypeTransactionProcessed)
	require.NoError(t, tp.CreateBlock(t))

	b := tp.GetLatestBlock()
	require.True(t, ContainsTransaction(b, tx))

	return b, tx
}

func TestNode_RecoverMissedBlocks(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system, WithUnicityCertificateValidator(&AlwaysValidCertificateValidator{}))
	defer tp.Close()
	//genesis := tp.GetLatestBlock()
	block1 := &block.Block{}
	require.NoError(t, json.Unmarshal([]byte(b1), &block1))
	block2 := &block.Block{}
	require.NoError(t, json.Unmarshal([]byte(b2), &block2))
	block3 := &block.Block{}
	require.NoError(t, json.Unmarshal([]byte(b3), &block3))

	tp.SubmitUnicityCertificate(block3.UnicityCertificate)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())

	require.Equal(t, recovering, tp.partition.status)
	ContainsEvent(t, tp, EventTypeRecoveryStarted)

	// make sure replication request is sent
	reqs := tp.mockNet.SentMessages(network.ProtocolLedgerReplicationReq)
	require.Equal(t, 1, len(reqs))
	require.IsType(t, reqs[0].Message, &replication.LedgerReplicationRequest{})
	req := reqs[0].Message.(*replication.LedgerReplicationRequest)

	require.Equal(t, req.GetBeginBlockNumber(), block1.GetBlockNumber())

	// send back the response
	tp.mockNet.Receive(network.ReceivedMessage{
		From:     reqs[0].ID,
		Protocol: network.ProtocolLedgerReplicationResp,
		Message: &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: []*block.Block{block1, block2, block3},
		},
	})

	ContainsEvent(t, tp, EventTypeRecoveryFinished)
	require.Equal(t, idle, tp.partition.status)
}

const (
	b1 string = `{
	"system_identifier": "AQEBAQ==",
	"block_number": 2,
	"previous_block_hash": "sITlQJ0pmBs0ehzOXiPp+fvIYgfuXPKAD1Zqbpq0PhY=",
	"transactions": [
		{
			"system_id": "AAAAAA==",
			"unit_id": "cw0+",
			"transaction_attributes": {
				"type_url": "type.googleapis.com/rpc.TransferOrder",
				"value": "CgO2a1IQ1uOex9rlt7HnARoDQTQ8"
			},
			"owner_proof": "gy93"
		}
	],
	"unicity_certificate": {
		"input_record": {
			"previous_hash": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"hash": "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"block_hash": "/i9LaJsTyrIGGubKpoKe8vxJk5H/RILCZ3cjEliobH4=",
			"summary_value": "AAAAAAAAAAA="
		},
		"unicity_tree_certificate": {
			"system_identifier": "AQEBAQ==",
			"sibling_hashes": [
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			],
			"system_description_hash": "JW2PJK6s0MbsGBDeJxHAixZYH19NkEETPaaG7eHwVKY="
		},
		"unicity_seal": {
			"root_chain_round_number": 2,
			"previous_hash": "zhTs7bDM/xg616uP6+xoEIdrpEs4kRA5LvmTjBX0ne4=",
			"hash": "m6YaKHsHdwS9Sp5eUJbbNvrrQu19qgigyyAc7X9V6Co=",
			"signature": "ZXIFDrJXUH0FjheKJOlMXdeGkBeo8BKf0n9PKBKABcYuN99EmPjD7wZl44+V9SrK9v/Igxfht/ml0iTmmG6IuAA="
		}
	}
}`
	b2 string = `{
	"system_identifier": "AQEBAQ==",
	"block_number": 3,
	"previous_block_hash": "6dZvFSBlqTzZ+YKmC2dvD7f9Yw/xMStTHyZLKAynCiw=",
	"transactions": [
		{
			"system_id": "AAAAAA==",
			"unit_id": "nH0j",
			"transaction_attributes": {
				"type_url": "type.googleapis.com/rpc.TransferOrder",
				"value": "CgMhhEAQ6+D0mqLu/NHjARoDGngU"
			},
			"owner_proof": "4nSn"
		}
	],
	"unicity_certificate": {
		"input_record": {
			"previous_hash": "AQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"hash": "AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"block_hash": "/cx6AWkpSxExf5kHH//LKoOgKpbXJl/sMEJfcBPw9yQ=",
			"summary_value": "AAAAAAAAAAA="
		},
		"unicity_tree_certificate": {
			"system_identifier": "AQEBAQ==",
			"sibling_hashes": [
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			],
			"system_description_hash": "JW2PJK6s0MbsGBDeJxHAixZYH19NkEETPaaG7eHwVKY="
		},
		"unicity_seal": {
			"root_chain_round_number": 3,
			"previous_hash": "m6YaKHsHdwS9Sp5eUJbbNvrrQu19qgigyyAc7X9V6Co=",
			"hash": "Q8cF2hmPjPP9LIq2tXs+oQFFJu7xV6CIZyYdr69exGs=",
			"signature": "2q40MfLQksMkoPG6atlhezEDVmEDkv4NHHbiOIYpjZQpeeIdqtX/Wxhmx+2GKD2pLOU+XQtBGuxdhmutwa0UrgE="
		}
	}
}`
	b3 string = `{
	"system_identifier": "AQEBAQ==",
	"block_number": 4,
	"previous_block_hash": "tbJ4uEb5CJxJUjuSzwGFLMDtoKTRNRclhzjxk2G5azU=",
	"transactions": [
		{
			"system_id": "AAAAAA==",
			"unit_id": "X+ua",
			"transaction_attributes": {
				"type_url": "type.googleapis.com/rpc.TransferOrder",
				"value": "CgP+MLoQrcSx17vqtamrARoDFV37"
			},
			"owner_proof": "QwNg"
		}
	],
	"unicity_certificate": {
		"input_record": {
			"previous_hash": "AwAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"hash": "BgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
			"block_hash": "yJbyvYwhpdy+IDd6jZlXaPxmmPvUi8v8g7Ffxe2Mwno=",
			"summary_value": "AAAAAAAAAAA="
		},
		"unicity_tree_certificate": {
			"system_identifier": "AQEBAQ==",
			"sibling_hashes": [
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
				"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
			],
			"system_description_hash": "JW2PJK6s0MbsGBDeJxHAixZYH19NkEETPaaG7eHwVKY="
		},
		"unicity_seal": {
			"root_chain_round_number": 4,
			"previous_hash": "Q8cF2hmPjPP9LIq2tXs+oQFFJu7xV6CIZyYdr69exGs=",
			"hash": "HnGDM8AZzuzA1mpxcOjSUHYTyoVUGWSGGcaZOsvMar4=",
			"signature": "ZA+tmB7EaGZLrciHECmvmIdfgFIzKbqsrYOwIoOo37JQvM7LgdS6LjhOXFnpdDjsVn0T7HKO4EtN3Z3paaFt7gA="
		}
	}
}`
)
