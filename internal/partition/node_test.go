package partition

import (
	gocrypto "crypto"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

type AlwaysValidCertificateValidator struct{}

type convertTxFailsTxSystem struct {
	testtxsystem.CounterTxSystem
}

func (c *convertTxFailsTxSystem) ConvertTx(*txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return nil, errors.New("invalid tx")
}

func TestNode_StartNewRoundCallsRInit(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p := NewSingleNodePartition(t, s)
	defer p.Close()
	ucr := &certificates.UnicityCertificate{
		InputRecord: &certificates.InputRecord{
			RoundNumber: 2,
		},
		UnicitySeal: &certificates.UnicitySeal{
			RootChainRoundNumber: 1,
			Hash:                 zeroHash,
		},
	}
	p.partition.startNewRound(ucr)
	require.Equal(t, uint64(1), s.BeginBlockCountDelta)
}

func TestNode_noRound_txAddedBackToBuffer(t *testing.T) {
	s := &testtxsystem.CounterTxSystem{}
	p := NewSingleNodePartition(t, s)
	defer p.Close()
	transfer := moneytesttx.RandomGenericBillTransfer(t)
	stateBefore, err := s.State()
	if err != nil {
		require.NoError(t, err)
	}
	bufferBefore := p.partition.txBuffer.Count()
	// make sure no round is active
	p.partition.handleT1TimeoutEvent()
	// send tx to the channel
	p.partition.txCh <- transfer
	// tx is added back to the buffer
	require.Eventually(t, func() bool {
		return bufferBefore+1 == p.partition.txBuffer.Count()
	}, test.WaitDuration, test.WaitTick)
	// make sure tx system remains untouched
	stateAfter, err := s.State()
	if err != nil {
		require.NoError(t, err)
	}
	require.Equal(t, stateBefore.Root(), stateAfter.Root())
}

func TestNode_HandleInvalidTxEvent(t *testing.T) {
	pn := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer pn.Close()
	message := network.ReceivedMessage{
		From:     "test-from",
		Protocol: network.ProtocolInputForward,
		Message:  &anypb.Any{},
	}
	err := pn.partition.handleTxMessage(message)
	require.ErrorContains(t, err, "unsupported type")
	require.Equal(t, 0, len(pn.partition.proposedTransactions))
}

func TestNode_ConvertingTxToGenericTxFails(t *testing.T) {
	system := &convertTxFailsTxSystem{}
	pn := NewSingleNodePartition(t, system)
	defer pn.Close()
	message := network.ReceivedMessage{
		From:     "test-from",
		Protocol: network.ProtocolInputForward,
		Message:  moneytesttx.RandomBillTransfer(t),
	}
	err := pn.partition.handleTxMessage(message)
	require.ErrorContains(t, err, "invalid tx")
	require.Equal(t, 0, len(pn.partition.proposedTransactions))
}

func TestNode_NodeStartTest(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	// node starts in init state
	require.Equal(t, initializing, tp.partition.status)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	// simulate no response, but monitor timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	// while no response is received a retry is triggered on each timeout
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	tp.SubmitMonitorTimeout(t)
	// node sends a handshake to root and subscribes to UC messages
	require.Eventually(t, RequestReceived(tp, network.ProtocolHandshake), 200*time.Millisecond, test.WaitTick)
	tp.mockNet.ResetSentMessages(network.ProtocolHandshake)
	// root responds with genesis
	tp.SubmitUnicityCertificate(tp.partition.luc)
	// node is initiated
	require.Eventually(t, func() bool {
		return tp.partition.status == normal
	}, test.WaitDuration, test.WaitTick)
}

func TestNode_CreateBlocks(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	tp.partition.startNewRound(tp.partition.luc)
	transfer := moneytesttx.RandomBillTransfer(t)
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
	require.True(t, ContainsTransaction(block1, transfer))

	tx1 := moneytesttx.RandomBillTransfer(t)
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
	tx2 := moneytesttx.RandomBillTransfer(t)
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
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	genesis := tp.GetLatestBlock(t)
	tp.partition.startNewRound(tp.partition.luc)
	require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block1 := tp.GetLatestBlock(t)
	require.NotEqual(t, genesis.UnicityCertificate.InputRecord.RoundNumber, block1.UnicityCertificate.InputRecord.RoundNumber)
	require.NotEqual(t, genesis.UnicityCertificate.InputRecord.BlockHash, block1.UnicityCertificate.InputRecord.BlockHash)

	// next block (empty)
	tp.CreateBlock(t)
	block2 := tp.GetLatestBlock(t) // this returns same block1 since empty block is not persisted
	require.Equal(t, block1, block2)
	// latest UC certifies empty block
	uc2 := tp.partition.luc
	require.Less(t, block2.UnicityCertificate.InputRecord.RoundNumber, uc2.InputRecord.RoundNumber)
	// hash of the latest certified empty block is zero-hash
	require.Equal(t, uc2.InputRecord.BlockHash, zeroHash)
	// state hash must stay the same as in last non-empty block
	require.Equal(t, block2.UnicityCertificate.InputRecord.Hash, uc2.InputRecord.Hash)

	// next block (empty)
	tp.CreateBlock(t)
	require.Equal(t, block1, tp.GetLatestBlock(t))
	uc3 := tp.partition.luc
	require.Less(t, uc2.InputRecord.RoundNumber, uc3.InputRecord.RoundNumber)
	require.Equal(t, uc3.InputRecord.BlockHash, zeroHash)
	require.Equal(t, block1.UnicityCertificate.InputRecord.Hash, uc3.InputRecord.Hash)

	// next block (non-empty)
	require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)
	block4 := tp.GetLatestBlock(t)
	require.NotEqual(t, block1, block4)
	require.NotEqual(t, block4.UnicityCertificate.InputRecord.BlockHash, zeroHash)
	require.Equal(t, block1.UnicityCertificate.InputRecord.BlockHash, block4.PreviousBlockHash)
	uc4 := tp.partition.luc
	require.Equal(t, block4.UnicityCertificate, uc4)
	require.Equal(t, block1.UnicityCertificate.InputRecord.Hash, uc4.InputRecord.PreviousHash)
	require.Less(t, uc3.InputRecord.RoundNumber, uc4.InputRecord.RoundNumber)
}

func TestNode_HandleNilUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	tp.SubmitUnicityCertificate(nil)
	ContainsError(t, tp, "unicity certificate is nil")
}

func TestNode_HandleOlderUnicityCertificate(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	tp.SubmitUnicityCertificate(block.UnicityCertificate)
	ContainsError(t, tp, "new certificate is from older root round 1 than previous certificate 2")
}

func TestNode_StartNodeBehindRootchain_OK(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	systemId := p.SystemIdentifier(tp.nodeConf.GetSystemIdentifier())
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
	tp := NewSingleNodePartition(t, txSystem)
	defer tp.Close()
	block := tp.GetLatestBlock(t) // genesis block
	txSystem.Revert()             // revert the state of the tx system
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	//genericBlock, _ := block.ToGenericBlock(txSystem)
	//blockHash, _ := genericBlock.Hash(gocrypto.SHA256)
	//block2 := tp.GetLatestBlock()
	uc2 := tp.partition.luc
	require.Equal(t, block.UnicityCertificate.InputRecord.RoundNumber+1, uc2.InputRecord.RoundNumber)
	require.Equal(t, block.SystemIdentifier, uc2.UnicityTreeCertificate.SystemIdentifier)
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
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	block = tp.GetLatestBlock(t)
	require.NotNil(t, block)
	latestUC := tp.partition.luc

	ir := proto.Clone(latestUC.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)
	ir.BlockHash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(ir, latestUC.UnicitySeal.RootChainRoundNumber)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating certificate, different input records for same partition round")
}

func TestNode_HandleEquivocatingUnicityCertificate_SameIRPreviousHashDifferentIRHash(t *testing.T) {
	txs := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txs)
	defer tp.Close()
	genesisUC := tp.partition.luc
	tp.partition.startNewRound(genesisUC)
	block := tp.GetLatestBlock(t)
	txs.ExecuteCountDelta++ // so that the block is not considered empty
	require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)

	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	latestUC := tp.partition.luc
	ir := proto.Clone(latestUC.InputRecord).(*certificates.InputRecord)
	ir.Hash = test.RandomBytes(32)

	equivocatingUC, err := tp.CreateUnicityCertificate(
		ir,
		latestUC.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(equivocatingUC)
	ContainsError(t, tp, "equivocating certificate, different input records for same partition round 2")
}

// state does not change in case of no transactions in money partition
func TestNode_HandleUnicityCertificate_SameIR_DifferentBlockHash_StateReverted(t *testing.T) {
	txs := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, txs)
	defer tp.Close()
	genesisUC := tp.partition.luc
	tp.partition.startNewRound(genesisUC)
	require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	tp.CreateBlock(t)

	latestUC := tp.partition.luc
	require.NotEqual(t, genesisUC, latestUC)
	tp.mockNet.ResetSentMessages(network.ProtocolBlockCertification)
	tp.partition.startNewRound(tp.partition.luc)
	// create a new transaction
	require.NoError(t, tp.SubmitTx(moneytesttx.RandomBillTransfer(t)))
	testevent.ContainsEvent(t, tp.eh, event.TransactionProcessed)
	// create block proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), txs.RevertCount)

	// simulate receiving repeat UC
	ir, _ := certificates.NewRepeatInputRecord(latestUC.InputRecord)
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
	tp := NewSingleNodePartition(t, txSystem)
	defer tp.Close()
	block := tp.GetLatestBlock(t)

	txSystem.EndBlockCount = 10000

	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
	ir.RoundNumber++
	uc, err := tp.CreateUnicityCertificate(
		ir,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)

	tp.SubmitUnicityCertificate(uc)

	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), txSystem.RevertCount)
	require.Equal(t, recovering, tp.partition.status)
}

// proposal not nil
// uc.InputRecord.Hash != n.pr.StateHash
// uc.InputRecord.Hash == n.pr.PrevHash
// => UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
func TestNode_HandleUnicityCertificate_Revert(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{EndBlockChangesState: true}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	block := tp.GetLatestBlock(t)

	transfer := moneytesttx.RandomBillTransfer(t)
	require.NoError(t, tp.SubmitTx(transfer))

	// create block proposal
	tp.SubmitT1Timeout(t)
	require.Equal(t, uint64(0), system.RevertCount)

	// send repeat UC
	ir := proto.Clone(block.UnicityCertificate.InputRecord).(*certificates.InputRecord)
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

func TestBlockProposal_BlockProposalIsNil(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	tp.SubmitBlockProposal(nil)
	ContainsError(t, tp, blockproposal.ErrBlockProposalIsNil.Error())
}

func TestBlockProposal_InvalidNodeIdentifier(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: "1", UnicityCertificate: block.UnicityCertificate})
	ContainsError(t, tp, "public key for id 1 not found")
}

func TestBlockProposal_InvalidBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)
	ver, err := tp.rootSigner.Verifier()
	require.NoError(t, err)
	rootTrust := map[string]crypto.Verifier{"test": ver}
	val, err := NewDefaultBlockProposalValidator(tp.nodeConf.genesis.SystemDescriptionRecord, rootTrust, gocrypto.SHA256)
	require.NoError(t, err)
	tp.partition.blockProposalValidator = val
	tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: tp.nodeDeps.peer.ID().String(), UnicityCertificate: block.UnicityCertificate})

	ContainsError(t, tp, "invalid system identifier")
}

func TestBlockProposal_HandleOldBlockProposal(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	transfer := moneytesttx.RandomBillTransfer(t)

	require.NoError(t, tp.SubmitTx(transfer))
	tp.CreateBlock(t)
	require.Eventually(t, NextBlockReceived(t, tp, block), test.WaitDuration, test.WaitTick)

	tp.SubmitBlockProposal(&blockproposal.BlockProposal{NodeIdentifier: tp.nodeDeps.peer.ID().String(), SystemIdentifier: tp.nodeConf.GetSystemIdentifier(), UnicityCertificate: block.UnicityCertificate})

	ContainsError(t, tp, "received UC is older, uc round 1, luc round 2")
}

func TestBlockProposal_ExpectedLeaderInvalid(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber+1,
	)
	require.NoError(t, err)
	tp.partition.leaderSelector = &TestLeaderSelector{
		leader:      "12",
		currentNode: "12",
	}
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peer.ID().String(),
		UnicityCertificate: uc,
		Transactions:       []*txsystem.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	tp.SubmitBlockProposal(bp)

	ContainsError(t, tp, "invalid node identifier. leader from UC:")
}

func TestBlockProposal_Ok(t *testing.T) {
	tp := NewSingleNodePartition(t, &testtxsystem.CounterTxSystem{})
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peer.ID().String(),
		UnicityCertificate: uc,
		Transactions:       []*txsystem.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	tp.SubmitBlockProposal(bp)
	require.Eventually(t, RequestReceived(tp, network.ProtocolBlockCertification), test.WaitDuration, test.WaitTick)
}

func TestBlockProposal_TxSystemStateIsDifferent_sameUC(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	uc, err := tp.CreateUnicityCertificate(
		block.UnicityCertificate.InputRecord,
		block.UnicityCertificate.UnicitySeal.RootChainRoundNumber,
	)
	require.NoError(t, err)
	bp := &blockproposal.BlockProposal{
		SystemIdentifier:   uc.UnicityTreeCertificate.SystemIdentifier,
		NodeIdentifier:     tp.nodeDeps.peer.ID().String(),
		UnicityCertificate: uc,
		Transactions:       []*txsystem.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	tp.SubmitBlockProposal(bp)
	ContainsError(t, tp, "tx system start state mismatch error, expected")
}

func TestBlockProposal_TxSystemStateIsDifferent_newUC(t *testing.T) {
	system := &testtxsystem.CounterTxSystem{}
	tp := NewSingleNodePartition(t, system)
	defer tp.Close()
	block := tp.GetLatestBlock(t)
	// create a UC for a new round
	ir := &certificates.InputRecord{
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
		NodeIdentifier:     tp.nodeDeps.peer.ID().String(),
		UnicityCertificate: uc,
		Transactions:       []*txsystem.Transaction{},
	}
	err = bp.Sign(gocrypto.SHA256, tp.nodeConf.signer)
	require.NoError(t, err)
	system.InitCount = 10000
	tp.SubmitBlockProposal(bp)
	ContainsError(t, tp, ErrNodeDoesNotHaveLatestBlock.Error())
	require.Equal(t, uint64(1), system.RevertCount)
	testevent.ContainsEvent(t, tp.eh, event.StateReverted)
	require.Equal(t, recovering, tp.partition.status)
}

func (c *AlwaysValidCertificateValidator) Validate(_ *certificates.UnicityCertificate) error {
	return nil
}
