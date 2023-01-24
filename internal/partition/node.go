package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/txbuffer"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	idle status = iota
	recovering
)

const t1TimerName = "t1"

var (
	ErrNodeDoesNotHaveLatestBlock = errors.New("node does not have the latest block")
	ErrStateReverted              = errors.New("state reverted")

	transactionsCounter = metrics.GetOrRegisterCounter("partition/node/transaction/handled")
)

type (

	// Net provides an interface for sending messages to and receiving messages from other nodes in the network.
	Net interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	// Node represents a member in the partition and implements an instance of a specific TransactionSystem. Partition
	// is a distributed system, it consists of either a set of shards, or one or more partition nodes.
	Node struct {
		status                      status
		configuration               *configuration
		transactionSystem           txsystem.TransactionSystem
		luc                         *certificates.UnicityCertificate
		proposedTransactions        []txsystem.GenericTransaction
		pendingBlockProposal        *block.PendingBlockProposal
		timers                      *timer.Timers
		leaderSelector              LeaderSelector
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  store.BlockStore
		txBuffer                    *txbuffer.TxBuffer
		ctx                         context.Context
		ctxCancel                   context.CancelFunc
		network                     Net
		txCancel                    context.CancelFunc
		txWaitGroup                 *sync.WaitGroup
		txCh                        chan txsystem.GenericTransaction
		eventCh                     chan event.Event
		eventChCancel               chan bool
		eventHandler                event.Handler
	}

	status int
)

// New creates a new instance of the partition node. All parameters expect the nodeOptions are required. Functions
// implementing the NodeOption interface can be used to override default configuration values:
//
//   n, err := New(
//  	peer,
// 		signer,
//		txSystem,
//		genesis,
// 		net,
// 		WithContext(context.Background()),
// 		WithTxValidator(myTxValidator)),
// 		WithUnicityCertificateValidator(ucValidator),
// 		WithBlockProposalValidator(blockProposalValidator),
//		WithLeaderSelector(leaderSelector),
// 		WithBlockStore(blockStore),
// 		WithT1Timeout(250*time.Millisecond),
//   )
//
// The following restrictions apply to the inputs:
// 		* the network peer and signer must use the same keys that were used to generate node genesis file;
//		* the state of the transaction system must be equal to the state that was used to generate genesis file.
func New(
	peer *network.Peer, // P2P peer for the node
	signer crypto.Signer, // used to sign block proposals and block certification requests
	txSystem txsystem.TransactionSystem, // used transaction system
	genesis *genesis.PartitionGenesis, // partition genesis file. created by rootchain.
	net Net, // network layer of the node
	nodeOptions ...NodeOption, // additional optional configuration parameters
) (*Node, error) {
	// load and validate node configuration
	conf, err := loadAndValidateConfiguration(peer, signer, genesis, txSystem, net, nodeOptions...)
	if err != nil {
		return nil, err
	}

	log.SetContext(log.KeyNodeID, conf.peer.ID().String())

	n := &Node{
		status:                      idle,
		configuration:               conf,
		transactionSystem:           txSystem,
		leaderSelector:              conf.leaderSelector,
		txValidator:                 conf.txValidator,
		unicityCertificateValidator: conf.unicityCertificateValidator,
		blockProposalValidator:      conf.blockProposalValidator,
		blockStore:                  conf.blockStore,
		txBuffer:                    conf.txBuffer,
		eventHandler:                conf.eventHandler,
		network:                     net,
		txWaitGroup:                 &sync.WaitGroup{},
	}
	n.ctx, n.ctxCancel = context.WithCancel(conf.context)

	// init timer
	n.timers = timer.NewTimers()
	n.timers.Start(t1TimerName, conf.t1Timeout)
	n.txCh = make(chan txsystem.GenericTransaction, conf.txBuffer.Capacity())
	if n.eventHandler != nil {
		n.eventCh = make(chan event.Event, conf.eventChCapacity)
		n.eventChCancel = make(chan bool)
		go n.eventHandlerLoop()
	}

	if err = initState(n); err != nil {
		return nil, err
	}

	go n.loop()

	go n.handshakeLoop()

	return n, nil
}

func (n *Node) handshakeLoop() {
	for {
		select {
		case <-n.ctx.Done():
			return
		default:
			logger.Trace("Sending handshake to root chain")
			err := n.network.Send(network.OutputMessage{
				Protocol: network.ProtocolHandshake,
				Message: &handshake.Handshake{
					SystemIdentifier: n.configuration.GetSystemIdentifier(),
					NodeIdentifier:   n.leaderSelector.SelfID().String(),
				},
			}, []peer.ID{n.configuration.rootChainID})

			if err != nil {
				logger.Error("error sending handshake", err)
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

func initState(n *Node) error {
	defer trackExecutionTime(time.Now(), "Restore node state")
	// get genesis block from the genesis
	genesisBlock := n.configuration.genesisBlock()
	// latest block from the store
	latestPersistedBlock, err := n.blockStore.LatestBlock()
	if err != nil {
		return err
	}
	var uc *certificates.UnicityCertificate
	if latestPersistedBlock != nil && latestPersistedBlock.UnicityCertificate.InputRecord.RoundNumber > genesisBlock.UnicityCertificate.InputRecord.RoundNumber {
		// restore from store
		prevBlock := genesisBlock
		for i := genesisBlock.UnicityCertificate.InputRecord.RoundNumber + 1; i <= latestPersistedBlock.UnicityCertificate.InputRecord.RoundNumber; i++ {
			bl, err := n.blockStore.Get(i)
			if err != nil {
				return err
			}
			if !bytes.Equal(prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.PreviousBlockHash) {
				return errors.Errorf("state init failed, invalid blockchain (previous block #%v hash='%X', current block #%v backlink='%X')", prevBlock.UnicityCertificate.InputRecord.RoundNumber, prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.UnicityCertificate.InputRecord.RoundNumber, bl.PreviousBlockHash)
			}
			uc, err = n.applyBlock(i, bl)
			if err != nil {
				return err
			}
			prevBlock = bl
		}
		logger.Info("State initialised from persistent store up to block #%v", prevBlock.UnicityCertificate.InputRecord.RoundNumber)

		n.restoreBlockProposal(prevBlock)
	} else {
		if err := n.blockStore.AddGenesis(genesisBlock); err != nil {
			return err
		}
		n.transactionSystem.Commit() // commit everything from the genesis
		uc = genesisBlock.UnicityCertificate
		logger.Info("State initialised from the genesis block")
	}
	n.updateLUC(uc)
	return nil
}

func (n *Node) applyBlock(blockNr uint64, bl *block.Block) (*certificates.UnicityCertificate, error) {
	n.transactionSystem.BeginBlock(blockNr)
	for _, tx := range bl.Transactions {
		gtx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			return nil, err
		}
		if err = n.validateAndExecuteTx(gtx, blockNr); err != nil {
			return nil, err
		}
	}

	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return nil, err
	}
	uc := bl.UnicityCertificate
	if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
		return nil, errors.Errorf("invalid tx system state root of block #%v. expected: %X, got: %X", bl.UnicityCertificate.InputRecord.RoundNumber, uc.InputRecord.Hash, state.Root())
	} else if !bytes.Equal(uc.InputRecord.SummaryValue, state.Summary()) {
		return nil, errors.Errorf("recovery failed: IR's summary value is not equal to tx system summary value (%X vs %X)", uc.InputRecord.SummaryValue, state.Summary())
	}
	n.transactionSystem.Commit()
	return uc, nil
}

func (n *Node) restoreBlockProposal(prevBlock *block.Block) {
	proposal, err := n.blockStore.GetPendingProposal()
	if err != nil {
		logger.Error("Error fetching block proposal: %s", err)
	}
	if proposal == nil {
		logger.Info("Stored block proposal not found")
		return
	}
	logger.Info("Trying to restore block proposal")

	reportAndRevert := func(msg string, err error) {
		logger.Error(msg, err)
		n.revertState()
	}

	// make sure proposal extends the previous state
	if bytes.Equal(prevBlock.UnicityCertificate.InputRecord.Hash, proposal.PrevHash) {
		logger.Debug("Stored block proposal extends the previous state")
		blockNr := prevBlock.UnicityCertificate.InputRecord.RoundNumber + 1
		n.transactionSystem.BeginBlock(blockNr)
		for _, gtx := range proposal.Transactions {
			if err = n.validateAndExecuteTx(gtx, blockNr); err != nil {
				reportAndRevert("Error executing tx from block proposal: %s", err)
				return
			}
		}

		state, err := n.transactionSystem.EndBlock()
		if err != nil {
			reportAndRevert("Error in transaction system: %s", err)
			return
		}
		if !bytes.Equal(proposal.StateHash, state.Root()) {
			reportAndRevert("Block proposal recovery failed: %s", errors.Errorf(", invalid state (proposal's state hash: %X, current state hash: %X", proposal.StateHash, state.Root()))
			return
		}
		n.transactionSystem.Commit()
		n.handleT1TimeoutEvent()
	}
}

// Close shuts down the Node component.
func (n *Node) Close() {
	logger.Info("Shutting down node '%v'", n.configuration.peer.ID())
	n.ctxCancel()
	n.timers.WaitClose()
	n.txBuffer.Close()
	close(n.txCh)
	if n.eventHandler != nil {
		n.eventChCancel <- true
	}
}

// loop handles receivedMessages from different goroutines.
func (n *Node) loop() {
	for {
		select {
		case <-n.ctx.Done():
			logger.Info("Exiting partition node component main loop")
			return
		case tx, ok := <-n.txCh:
			if !ok {
				logger.Warning("Tx channel closed, exiting main loop")
				return
			}
			// round might not be active, but some transactions might still be in the channel
			if n.txCancel == nil {
				logger.Warning("No active round, adding tx back to the buffer, UnitID=%X", tx.UnitID().Bytes32())
				err := n.txBuffer.Add(tx)
				if err != nil {
					logger.Warning("Invalid transaction: %v", err)
					n.sendEvent(event.Error, err)
				}
			} else {
				n.process(tx)
			}
		case m, ok := <-n.network.ReceivedChannel():
			if !ok {
				logger.Warning("Received channel closed, exiting main loop")
				return
			}
			if m.Message == nil {
				logger.Warning("Received network message is nil")
				continue
			}
			switch m.Protocol {
			case network.ProtocolInputForward:
				err := n.handleTxMessage(m)
				if err != nil {
					logger.Warning("Invalid transaction: %v", err)
					n.sendEvent(event.Error, err)
				}
			case network.ProtocolUnicityCertificates:
				success, uc := convertType[*certificates.UnicityCertificate](m.Message)
				if !success {
					logger.Warning("Invalid unicity certificate type: %T", m.Message)
					continue
				}
				err := n.handleUnicityCertificate(uc)
				if err != nil {
					logger.Warning("Unicity Certificate processing failed: %v", err)
					n.sendEvent(event.Error, err)
					continue
				}
				n.sendEvent(event.UnicityCertificateHandled, uc)
			case network.ProtocolBlockProposal:
				success, bp := convertType[*blockproposal.BlockProposal](m.Message)
				if !success {
					logger.Warning("Invalid block proposal type: %T", m.Message)
					continue
				}
				err := n.handleBlockProposal(bp)
				if err != nil {
					logger.Warning("Block proposal processing failed by node %v: %v", n.configuration.peer.ID(), err)
					n.sendEvent(event.Error, err)
					continue
				}
			case network.ProtocolLedgerReplicationReq:
				success, lr := convertType[*replication.LedgerReplicationRequest](m.Message)
				if !success {
					logger.Warning("Invalid ledger replication request type: %T", m.Message)
					continue
				}
				if err := n.handleLedgerReplicationRequest(lr); err != nil {
					logger.Warning("Ledger replication failed by node %v: %v", n.configuration.peer.ID(), err)
				}
			case network.ProtocolLedgerReplicationResp:
				success, lr := convertType[*replication.LedgerReplicationResponse](m.Message)
				if !success {
					logger.Warning("Invalid ledger replication response type: %T", m.Message)
					continue
				}
				if err := n.handleLedgerReplicationResponse(lr); err != nil {
					logger.Warning("Ledger replication failed by node %v: %v", n.configuration.peer.ID(), err)
				}
			default:
				logger.Warning("Unknown network protocol: %s", m.Protocol)
			}
		case _, ok := <-n.timers.C:
			if !ok {
				logger.Warning("Timers channel closed, exiting main loop")
				return
			}
			logger.Info("Handling T1 timeout")
			n.handleT1TimeoutEvent()
		}
	}
}

func (n *Node) sendEvent(eventType event.Type, content any) {
	if n.eventHandler != nil {
		n.eventCh <- event.Event{
			EventType: eventType,
			Content:   content,
		}
	}
}

// eventHandlerLoop forwards events produced by a node to the configured eventHandler.
func (n *Node) eventHandlerLoop() {
	for {
		select {
		case <-n.eventChCancel:
			return
		case e := <-n.eventCh:
			n.eventHandler(&e)
		}
	}
}

func (n *Node) handleTxMessage(m network.ReceivedMessage) error {
	success, tx := convertType[*txsystem.Transaction](m.Message)
	if !success {
		return errors.Errorf("unsupported type: %T", m.Message)
	}
	genTx, err := n.transactionSystem.ConvertTx(tx)
	if err != nil {
		return err
	}
	return n.txBuffer.Add(genTx)
}

func (n *Node) startNewRound(uc *certificates.UnicityCertificate) error {
	if n.status == recovering {
		logger.Warning("Unable to start new round, node is recovering")
		return nil
	}
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return err
	}
	newBlockNr := rn + 1
	n.transactionSystem.BeginBlock(newBlockNr)
	n.proposedTransactions = []txsystem.GenericTransaction{}
	n.pendingBlockProposal = nil
	n.leaderSelector.UpdateLeader(uc.UnicitySeal)
	n.startHandleOrForwardTransactions()
	n.updateLUC(uc)
	n.timers.Restart(t1TimerName)
	n.sendEvent(event.NewRoundStarted, newBlockNr)
	return nil
}

func (n *Node) handleOrForwardTransaction(tx txsystem.GenericTransaction) bool {
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		logger.Warning("Unable to get latest round number: %v %v", rn, err)
		return false
	}
	if err := n.txValidator.Validate(tx, rn); err != nil {
		logger.Warning("Received invalid transaction: %v", err)
		return true
	}
	leader := n.leaderSelector.GetLeaderID()
	if leader == n.leaderSelector.SelfID() {
		n.txCh <- tx
		return true
	}

	logger.Info("Forwarding tx %X to %v", tx.Hash(gocrypto.SHA256), leader)
	err = n.network.Send(
		network.OutputMessage{
			Protocol: network.ProtocolInputForward,
			Message:  tx.ToProtoBuf(),
		},
		[]peer.ID{leader},
	)
	// TODO unreported error?
	return err == nil
}

func (n *Node) process(tx txsystem.GenericTransaction) error {
	defer trackExecutionTime(time.Now(), "Processing transaction")
	bl, err := n.blockStore.LatestBlock()
	if err != nil {
		return fmt.Errorf("unable to get latest block from block store: %w", err)
	}
	if err := n.validateAndExecuteTx(tx, bl.UnicityCertificate.InputRecord.RoundNumber); err != nil {
		n.sendEvent(event.TransactionFailed, tx)
		return err
	}
	n.proposedTransactions = append(n.proposedTransactions, tx)
	n.sendEvent(event.TransactionProcessed, tx)
	logger.Debug("Transaction processed by node %v. Proposal size: %v", n.configuration.peer.ID(), len(n.proposedTransactions))
	return nil
}

func (n *Node) validateAndExecuteTx(tx txsystem.GenericTransaction, latestBlockNumber uint64) error {
	if err := n.txValidator.Validate(tx, latestBlockNumber); err != nil {
		logger.Warning("Transaction '%v' is invalid: %v", tx, err)
		return err
	}
	if err := n.transactionSystem.Execute(tx); err != nil {
		logger.Warning("TxSystem was unable to process transaction '%v': %v", tx, err)
		return err
	}
	return nil
}

// handleBlockProposal processes a block proposals. Performs the following steps:
//  1. Block proposal as a whole is validated:
// 		 * It must have valid signature, correct transaction system ID, valid UC;
//     	 * the UC must be not older than the latest known by current node;
//    	 * Sender must be the leader for the round started by included UC.
//  2. If included UC is newer than latest UC then the new UC is processed; this rolls back possible pending change in
//     the transaction system. If new UC is ‘repeat UC’ then update is reasonably fast; if recovery is necessary then
//     likely it takes some time and there is no reason to finish the processing of current proposal.
//  3. If the transaction system root is not equal to one extended by the processed proposal then processing is aborted.
//  4. All transaction orders in proposal are validated; on encountering an invalid transaction order the processing is
//     aborted.
//  5. Transaction orders are executed by applying them to the transaction system.
//  6. Pending unicity certificate request data structure is created and persisted.
//  7. Certificate Request query is assembled and sent to the Root Chain.
func (n *Node) handleBlockProposal(prop *blockproposal.BlockProposal) error {
	if n.status == recovering {
		logger.Warning("Ignoring block proposal, node is recovering")
		return nil
	}
	defer trackExecutionTime(time.Now(), "Handling BlockProposal")
	if prop == nil {
		return blockproposal.ErrBlockProposalIsNil
	}
	logger.Debug("Handling block proposal, its UC IR Hash %X, Block hash %X", prop.UnicityCertificate.InputRecord.Hash, prop.UnicityCertificate.InputRecord.BlockHash)
	nodeSignatureVerifier, err := n.configuration.GetSigningPublicKey(prop.NodeIdentifier)
	if err != nil {
		return err
	}
	if err := n.blockProposalValidator.Validate(prop, nodeSignatureVerifier); err != nil {
		logger.Warning("Block proposal is not valid: %v", err)
		return err
	}

	uc := prop.UnicityCertificate
	// UC must be newer than the last one seen
	if uc.UnicitySeal.RootChainRoundNumber < n.luc.UnicitySeal.RootChainRoundNumber {
		logger.Warning("Received UC is older than LUC. UC round Number:  %v, LUC round number: %v",
			uc.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
		return errors.Errorf("received UC is older than LUC. uc round %v, luc round %v",
			uc.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
	}
	expectedLeader := n.leaderSelector.LeaderFromUnicitySeal(uc.UnicitySeal)
	if expectedLeader == UnknownLeader || prop.NodeIdentifier != expectedLeader.String() {
		return errors.Errorf("invalid node identifier. leader from UC: %v, request leader: %v", expectedLeader, prop.NodeIdentifier)
	}

	logger.Debug("Proposal's UC root nr: %v vs LUC root nr: %v", uc.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
	if uc.UnicitySeal.RootChainRoundNumber > n.luc.UnicitySeal.RootChainRoundNumber {
		err := n.handleUnicityCertificate(uc)
		if err != nil && err != ErrStateReverted {
			return err
		}
	}
	prevHash := uc.InputRecord.Hash
	txState, err := n.transactionSystem.State()
	if err != nil {
		if err == txsystem.ErrStateContainsUncommittedChanges {
			return errors.Wrap(err, "tx system contains uncommitted changes")
		}
		return err
	}

	if !bytes.Equal(prevHash, txState.Root()) {
		return errors.Errorf("invalid tx system state root. expected: %X, got: %X", txState.Root(), prevHash)
	}
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return err
	}
	n.transactionSystem.BeginBlock(rn + 1)
	for _, tx := range prop.Transactions {
		genTx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			logger.Warning("transaction is invalid %v", err)
			return err
		}
		err = n.process(genTx)
		if err != nil {
			return fmt.Errorf("transaction processing failed %v", err)
		}
	}
	return n.sendCertificationRequest()
}

// handleUnicityCertificate processes the Unicity Certificate and finalizes a block. Performs the following steps:
// 	1. Given UC is validated cryptographically.
//  2. Given UC must be newer than the last one seen.
// 	3. Given UC is checked for equivocation, that is,
//	   a) there can not be two UC-s with the same Root Chain block number but certifying different state root hashes;
//	   b) there can not be two UC-s extending the same state, but certifying different states (forking).
//  4. On unexpected case where there is no pending block proposal, recovery is initiated, unless the state is already
//     up-to-date with the given UC.
//  5. Alternatively, if UC certifies the pending block proposal then block is finalized.
//  6. Alternatively, if UC certifies the IR before pending block proposal (‘repeat UC’) then
//     state is rolled back to previous state.
//  7. Alternatively, recovery is initiated, after rollback. Note that recovery may end up with
//     newer last known UC than the one being processed.
//  8. New round is started.
func (n *Node) handleUnicityCertificate(uc *certificates.UnicityCertificate) error {
	defer trackExecutionTime(time.Now(), "Handling unicity certificate")
	util.WriteDebugJsonLog(logger, "Handle Unicity Certificate", uc)
	// UC is validated cryptographically
	if err := n.unicityCertificateValidator.Validate(uc); err != nil {
		logger.Warning("Invalid UnicityCertificate: %v", err)
		return errors.Errorf("invalid unicity certificate: %v", err)
	}
	logger.Debug("Received Unicity Certificate: \nIR Hash: \t\t%X, \nIR Prev Hash: \t%X, \nBlock hash: \t%X", uc.InputRecord.Hash, uc.InputRecord.PreviousHash, uc.InputRecord.BlockHash)
	logger.Debug("LUC:                          \nIR Hash: \t\t%X, \nIR Prev Hash: \t%X, \nBlock hash: \t%X", n.luc.InputRecord.Hash, n.luc.InputRecord.PreviousHash, n.luc.InputRecord.BlockHash)
	if n.pendingBlockProposal != nil {
		pr := n.pendingBlockProposal
		logger.Debug("Pending proposal: \nstate hash:\t%X, \nprev hash: \t%X, \nroot round: %v, tx count: %v", pr.StateHash, pr.PrevHash, pr.RoundNumber, len(pr.Transactions))
	}
	// UC must be newer than the last one seen
	if uc.UnicitySeal.RootChainRoundNumber < n.luc.UnicitySeal.RootChainRoundNumber {
		logger.Warning("Received UC is older than LUC. UC round Number:  %v, LUC round number: %v",
			uc.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
		return errors.Errorf("received UC is older than LUC. uc round %v, luc round %v",
			uc.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
	}

	// there can not be two UC-s with the same Root Chain block number but certifying different state root hashes.
	if uc.UnicitySeal.RootChainRoundNumber == n.luc.UnicitySeal.RootChainRoundNumber &&
		!bytes.Equal(uc.InputRecord.Hash, n.luc.InputRecord.Hash) {
		logger.Warning("Got two UC-s with the same Base Chain block number but certifying different state root "+
			"hashes. RootChainNumber: %v, UC IR hash: %X, LUC IR hash: %X",
			uc.UnicitySeal.RootChainRoundNumber,
			uc.InputRecord.Hash,
			n.luc.InputRecord.Hash,
		)
		return errors.Errorf("equivocating certificates: round number %v, received IR hash %X, latest IR hash %X",
			uc.UnicitySeal.RootChainRoundNumber, uc.InputRecord.Hash, n.luc.InputRecord.Hash)
	}

	// there can not be two UC-s extending the same state, but certifying different states (forking).
	if bytes.Equal(uc.InputRecord.PreviousHash, n.luc.InputRecord.PreviousHash) &&
		!bytes.Equal(n.luc.InputRecord.PreviousHash, n.luc.InputRecord.Hash) && // exclude empty blocks
		!bytes.Equal(uc.InputRecord.Hash, n.luc.InputRecord.Hash) {
		logger.Warning("Got two UC-s extending the same state, but certifying different states. "+
			"PreviousHash: %X, UC IR hash: %X, LUC IR hash: %X",
			uc.InputRecord.PreviousHash,
			uc.InputRecord.Hash,
			n.luc.InputRecord.Hash,
		)
		return errors.Errorf("equivocating certificates. previous IR hash %X, received IR hash %X, "+
			"latest IR hash %X", uc.InputRecord.PreviousHash, uc.InputRecord.Hash, n.luc.InputRecord.Hash)
	}

	if n.pendingBlockProposal == nil {
		// There is no pending block proposal. Start recovery unless the state is already up-to-date with UC.
		state, err := n.transactionSystem.EndBlock()
		if err != nil {
			return errors.Wrap(err, "tx system failed to end block")
		}
		if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			logger.Warning("UC IR hash not equal to state's hash: '%X' vs '%X'", uc.InputRecord.Hash, state.Root())
			return n.startRecovery(uc)
		} else if latestUC, err := n.blockStore.LatestUC(); !bytes.Equal(uc.InputRecord.BlockHash, latestUC.InputRecord.BlockHash) {
			if err != nil {
				return errors.Wrap(err, "failed fetching latest UC")
			}
			logger.Warning("Received UC IR block hash not equal to latest round's (#%v) block hash: '%X' vs '%X'", latestUC.InputRecord.RoundNumber, uc.InputRecord.BlockHash, latestUC.InputRecord.BlockHash)
			return n.startRecovery(uc)
		} else {
			logger.Debug("No pending block proposal, UC IR hash is equal to State hash, so are block hashes")
			// if node is still recovering, this might be a good point to stop as the latest UC proves it's finished
			if n.status == recovering {
				n.stopRecovery(uc)
			}
		}
	} else if bl, blockHash, err := n.proposalHash(n.pendingBlockProposal.Transactions, uc); bytes.Equal(uc.InputRecord.Hash, n.pendingBlockProposal.StateHash) && bytes.Equal(uc.InputRecord.BlockHash, blockHash) {
		if err != nil {
			return errors.Wrap(err, "block finalization: proposal hash calculation failed")
		}
		// UC certifies pending block proposal
		err = n.finalizeBlock(bl)
		if err != nil {
			return errors.Wrap(err, "block finalization failed")
		}
	} else if bytes.Equal(uc.InputRecord.Hash, n.pendingBlockProposal.PrevHash) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		logger.Warning("Reverting state tree. UC IR hash: %X, proposal hash %X", uc.InputRecord.Hash, n.pendingBlockProposal.PrevHash)
		n.revertState()
		return errors.Wrap(n.startNewRound(uc), ErrStateReverted.Error())
	} else {
		// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
		return n.startRecovery(uc)
	}

	return n.startNewRound(uc)
}

func (n *Node) revertState() {
	logger.Warning("Reverting state")
	n.sendEvent(event.StateReverted, nil)
	n.transactionSystem.Revert()
}

func (n *Node) proposalHash(transactions []txsystem.GenericTransaction, uc *certificates.UnicityCertificate) (*block.Block, []byte, error) {
	latestBlock, err := n.blockStore.LatestBlock()
	if err != nil {
		return nil, nil, err
	}
	b := &block.GenericBlock{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		// latest non-empty block
		PreviousBlockHash:  latestBlock.UnicityCertificate.InputRecord.BlockHash,
		Transactions:       transactions,
		UnicityCertificate: uc,
	}
	blockHash, err := b.Hash(n.configuration.hashAlgorithm)
	if err != nil {
		return nil, nil, err
	}
	logger.Debug("Proposal hash: %X", blockHash)
	return b.ToProtobuf(), blockHash, nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(b *block.Block) error {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("Block #%v finalization", b.UnicityCertificate.InputRecord.RoundNumber))
	err := n.blockStore.Add(b)
	if err != nil {
		return err
	}
	n.transactionSystem.Commit()
	transactionsCounter.Inc(int64(len(b.Transactions)))
	n.sendEvent(event.BlockFinalized, b)
	return nil
}

func (n *Node) handleT1TimeoutEvent() {
	n.stopForwardingOrHandlingTransactions()
	defer func() {
		n.leaderSelector.UpdateLeader(nil)
	}()
	if n.status == recovering {
		logger.Info("T1 timeout: node is recovering")
		return
	}
	if n.leaderSelector.IsCurrentNodeLeader() {
		logger.Debug("Current node is the leader.")
		if err := n.sendBlockProposal(); err != nil {
			logger.Warning("Failed to send BlockProposal: %v", err)
			return
		}
		if err := n.sendCertificationRequest(); err != nil {
			logger.Warning("Failed to send certification request: %v", err)
		}
	} else {
		logger.Debug("Current node is not the leader.")
	}
}

func (n *Node) handleLedgerReplicationRequest(lr *replication.LedgerReplicationRequest) error {
	util.WriteDebugJsonLog(logger, "Ledger replication request received", lr)

	recoveringNodeID, err := peer.Decode(lr.NodeIdentifier)
	if err != nil {
		return errors.Errorf("failed to decode Peer ID: %s", lr.NodeIdentifier)
	}

	// TODO: check recoveringNodeID is among known validators
	// n.configuration.peer.Validators()

	maxBlock, err := n.blockStore.BlockNumber()
	if err != nil {
		return errors.Wrap(err, "unable to fetch block number from the block store")
	}

	resp := &replication.LedgerReplicationResponse{
		Status: replication.LedgerReplicationResponse_OK,
	}

	if !bytes.Equal(lr.SystemIdentifier, n.configuration.GetSystemIdentifier()) {
		resp.Status = replication.LedgerReplicationResponse_UNKNOWN_SYSTEM_IDENTIFIER
		resp.Message = fmt.Sprintf("Unknown system identifier: %v", lr.GetSystemIdentifier())
	}

	startBlock := lr.BeginBlockNumber
	if maxBlock < startBlock {
		resp.Status = replication.LedgerReplicationResponse_BLOCKS_NOT_FOUND
		resp.Message = fmt.Sprintf("Unknown block to form recovery response: %v, latest block: %v", startBlock, maxBlock)
	} else if lr.EndBlockNumber > startBlock {
		maxBlock = lr.EndBlockNumber
	}
	endBlock := util.Min(maxBlock, startBlock+n.configuration.replicationConfig.maxBlocks-1)
	logger.Debug("Preparing replication response from block #%v up to #%v", startBlock, endBlock)

	go func() {
		var countTx uint32 = 0
		if resp.Status == replication.LedgerReplicationResponse_OK {
			var blocks []*block.Block
			for i := startBlock; i <= endBlock; i++ {
				b, _ := n.blockStore.Get(i)
				blocks = append(blocks, b)
				countTx += uint32(len(b.Transactions))
				if countTx >= n.configuration.replicationConfig.maxTx {
					break
				}
			}
			resp.Blocks = blocks
		}
		err := n.network.Send(network.OutputMessage{
			Protocol: network.ProtocolLedgerReplicationResp,
			Message:  resp,
		}, []peer.ID{recoveringNodeID})
		if err != nil {
			logger.Error("Problem sending ledger replication response, %s: %s", resp.Pretty(), err)
		}
		n.sendEvent(event.ReplicationResponseSent, resp)
	}()

	return nil
}

func (n *Node) handleLedgerReplicationResponse(lr *replication.LedgerReplicationResponse) error {
	logger.Debug("Ledger replication response received: %s, ", lr.Pretty())
	bl, err := n.GetLatestBlock()
	if err != nil {
		return err
	}
	logger.Debug("Recovery: latest node's block: #%v", bl.UnicityCertificate.InputRecord.RoundNumber)
	rn, err := n.GetLatestRoundNumber()
	if err != nil {
		return err
	}
	logger.Debug("Recovery: latest round number: #%v", rn)

	if n.status != recovering {
		logger.Warning("Unexpected Ledger Replication response, node is not recovering: %s", lr.Pretty())
		return nil
	}

	if lr.Status != replication.LedgerReplicationResponse_OK {
		recoverFrom := rn + 1
		logger.Debug("Resending replication request starting with round #%v", recoverFrom)
		go func() {
			time.Sleep(500 * time.Millisecond) // TODO
			n.sendLedgerReplicationRequest(recoverFrom)
		}()
		return errors.Errorf("got erroneous Ledger Replication response, status=%s, message='%s'", lr.Status.String(), lr.Message)
	}
	for _, b := range lr.Blocks {
		logger.Debug("Recovering block #%v", b.UnicityCertificate.InputRecord.RoundNumber)
		if !bytes.Equal(b.SystemIdentifier, n.configuration.GetSystemIdentifier()) {
			return errors.Errorf("recovery failed: block %v contains invalid System ID: %x", b.UnicityCertificate.InputRecord.RoundNumber, b.SystemIdentifier)
		}

		_, err := n.applyBlock(b.UnicityCertificate.InputRecord.RoundNumber, b)
		if err != nil {
			n.revertState()
			latestUC, err := n.blockStore.LatestUC()
			if err != nil {
				err = errors.Wrap(err, "failed to fetch latest UC from the block store")
			}
			n.stopRecovery(latestUC)
			return errors.Wrapf(err, "recovery failed")
		}

		if err = n.blockStore.Add(b); err != nil {
			return err
		}
	}

	// check if recovery is complete
	latestUC, err := n.blockStore.LatestUC()
	if err != nil {
		return err
	}
	//latestBlock := n.GetLatestBlock()
	logger.Debug("Checking if recovery is complete, latest round: #%v", latestUC.InputRecord.RoundNumber)
	if latestUC.UnicitySeal.RootChainRoundNumber >= n.luc.UnicitySeal.RootChainRoundNumber {
		n.updateLUC(latestUC)
		n.stopRecovery(n.luc)
	} else {
		logger.Debug("Not fully recovered yet, latest block's UC root round %v vs LUC's root round %v", latestUC.UnicitySeal.RootChainRoundNumber, n.luc.UnicitySeal.RootChainRoundNumber)
		go n.sendLedgerReplicationRequest(latestUC.InputRecord.RoundNumber + 1)
	}
	return nil
}

func (n *Node) stopRecovery(uc *certificates.UnicityCertificate) {
	logger.Info("Node is recovered until the given UC, block '%X', root round: %v ", uc.InputRecord.BlockHash, uc.UnicitySeal.RootChainRoundNumber)
	n.status = idle
	n.sendEvent(event.RecoveryFinished, uc)
}

func (n *Node) startRecovery(uc *certificates.UnicityCertificate) error {
	if n.status == recovering {
		// already recovering, but if uc is newer than luc, let's update luc
		if uc.UnicitySeal.RootChainRoundNumber > n.luc.UnicitySeal.RootChainRoundNumber {
			logger.Warning("Recovery in progress, but received a newer UC, updating LUC")
			n.updateLUC(uc)
		}
		return nil
	}
	n.revertState()
	logger.Warning("Starting recovery")
	n.status = recovering
	n.stopForwardingOrHandlingTransactions()
	n.luc = uc // recover up to this UC
	util.WriteDebugJsonLog(logger, "Recovering node up to the given LUC", n.luc)
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return err
	}
	fromBlockNr := rn + 1
	n.sendEvent(event.RecoveryStarted, fromBlockNr)

	go n.sendLedgerReplicationRequest(fromBlockNr)

	return ErrNodeDoesNotHaveLatestBlock
}

func (n *Node) updateLUC(uc *certificates.UnicityCertificate) {
	if n.luc != nil && uc.UnicitySeal.RootChainRoundNumber <= n.luc.UnicitySeal.RootChainRoundNumber {
		return
	}
	n.luc = uc
	logger.Info("Updated LUC, root round: %v", n.luc.UnicitySeal.RootChainRoundNumber)
	n.sendEvent(event.LatestUnicityCertificateUpdated, uc)
}

func (n *Node) sendLedgerReplicationRequest(startingBlockNr uint64) {
	req := &replication.LedgerReplicationRequest{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		NodeIdentifier:   n.leaderSelector.SelfID().String(),
		BeginBlockNumber: startingBlockNr,
	}

	util.WriteDebugJsonLog(logger, "Ledger replication request created", req)

	peers := n.configuration.peer.Validators()
	if len(peers) == 0 {
		logger.Error("Error sending ledger replication request: %s", errors.Errorf("unable to send ledger replication request, no peers"))
	}

	var err error
	// send Ledger Replication request to a first alive randomly chosen node
	for _, p := range util.ShuffleSliceCopy(peers) {
		logger.Debug("Sending ledger replication request to peer '%v'", p)
		err = n.network.Send(network.OutputMessage{
			Protocol: network.ProtocolLedgerReplicationReq,
			Message:  req,
		}, []peer.ID{p})

		if err == nil {
			break
		}
	}

	if err != nil {
		logger.Warning("Error sending ledger replication request: %s", err)
	}
}

func (n *Node) sendBlockProposal() error {
	defer trackExecutionTime(time.Now(), "Sending BlockProposal")
	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.leaderSelector.SelfID()
	prop := &blockproposal.BlockProposal{
		SystemIdentifier:   systemIdentifier,
		NodeIdentifier:     nodeId.String(),
		UnicityCertificate: n.luc,
		Transactions:       toProtoBuf(n.proposedTransactions),
	}
	util.WriteDebugJsonLog(logger, "BlockProposal created", prop)
	err := prop.Sign(n.configuration.hashAlgorithm, n.configuration.signer)
	if err != nil {
		return err
	}

	return n.network.Send(network.OutputMessage{
		Protocol: network.ProtocolBlockProposal,
		Message:  prop,
	}, n.configuration.peer.Validators())
}

func (n *Node) sendCertificationRequest() error {
	defer trackExecutionTime(time.Now(), "Sending CertificationRequest")
	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.leaderSelector.SelfID()
	prevStateHash := n.luc.InputRecord.Hash
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return errors.Wrap(err, "tx system failed to end block")
	}
	stateHash := state.Root()
	summary := state.Summary()

	pendingProposal := &block.PendingBlockProposal{
		RoundNumber:  n.luc.UnicitySeal.RootChainRoundNumber,
		PrevHash:     prevStateHash,
		StateHash:    stateHash,
		Transactions: n.proposedTransactions,
	}
	err = n.blockStore.AddPendingProposal(pendingProposal)
	if err != nil {
		return errors.Wrap(err, "failed to store pending block proposal")
	}
	n.pendingBlockProposal = pendingProposal

	latestBlock, err := n.blockStore.LatestBlock()
	if err != nil {
		return err
	}
	latestBlockHash := latestBlock.UnicityCertificate.InputRecord.BlockHash
	blockHash, err := n.hashProposedBlock(latestBlockHash)
	if err != nil {
		return err
	}
	n.proposedTransactions = []txsystem.GenericTransaction{}

	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return err
	}
	req := &certification.BlockCertificationRequest{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId.String(),
		RootRoundNumber:  n.pendingBlockProposal.RoundNumber,
		InputRecord: &certificates.InputRecord{
			PreviousHash: prevStateHash,
			Hash:         stateHash,
			BlockHash:    blockHash,
			SummaryValue: summary,
			// latestBlock is the latest non-empty block,
			// latest UC might have certified an empty block and has the latest round number
			RoundNumber: rn + 1,
		},
	}
	err = req.Sign(n.configuration.signer)
	if err != nil {
		return err
	}
	logger.Info("Sending block #%v certification request to root chain, IR hash %X, Block Hash %X, rc nr: %v", latestBlock.UnicityCertificate.InputRecord.RoundNumber+1, stateHash, blockHash, req.RootRoundNumber)
	util.WriteDebugJsonLog(logger, "Sending block certification request to root chain", req)

	return n.network.Send(network.OutputMessage{
		Protocol: network.ProtocolBlockCertification,
		Message:  req,
	}, []peer.ID{n.configuration.rootChainID})
}

func (n *Node) SubmitTx(tx *txsystem.Transaction) error {
	genTx, err := n.transactionSystem.ConvertTx(tx)
	if err != nil {
		return err
	}
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return err
	}
	err = n.txValidator.Validate(genTx, rn)
	if err != nil {
		return err
	}
	return n.txBuffer.Add(genTx)
}

func (n *Node) GetBlock(blockNr uint64) (*block.Block, error) {
	return n.blockStore.Get(blockNr)
}

func (n *Node) GetLatestBlock() (*block.Block, error) {
	return n.blockStore.LatestBlock()
}

func (n *Node) GetLatestRoundNumber() (uint64, error) {
	rn, err := n.blockStore.LatestRoundNumber()
	if err != nil {
		return 0, err
	}
	return rn, nil
}

func (n *Node) SystemIdentifier() []byte {
	return n.configuration.GetSystemIdentifier()
}

func (n *Node) stopForwardingOrHandlingTransactions() {
	if n.txCancel != nil {
		txCancel := n.txCancel
		n.txCancel = nil
		txCancel()
		n.txWaitGroup.Wait()
	}
}

func (n *Node) startHandleOrForwardTransactions() {
	n.stopForwardingOrHandlingTransactions()
	leader := n.leaderSelector.GetLeaderID()
	if leader == UnknownLeader {
		return
	}
	txCtx, txCancel := context.WithCancel(context.Background())
	n.txCancel = txCancel
	n.txWaitGroup.Add(1)
	go n.txBuffer.Process(txCtx, n.txWaitGroup, n.handleOrForwardTransaction)
}

func (n *Node) hashProposedBlock(prevBlockHash []byte) ([]byte, error) {
	b := block.GenericBlock{
		SystemIdentifier:  n.configuration.GetSystemIdentifier(),
		PreviousBlockHash: prevBlockHash,
		Transactions:      n.pendingBlockProposal.Transactions,
	}
	return b.Hash(n.configuration.hashAlgorithm)
}

func convertType[T any](event interface{}) (bool, T) {
	var result T
	switch event.(type) {
	case T:
		return true, event.(T)
	}
	return false, result
}

func trackExecutionTime(start time.Time, name string) {
	logger.Debug("%s took %s", name, time.Since(start))
}

func toProtoBuf(transactions []txsystem.GenericTransaction) []*txsystem.Transaction {
	protoTransactions := make([]*txsystem.Transaction, len(transactions))
	for i, tx := range transactions {
		protoTransactions[i] = tx.ToProtoBuf()
	}
	return protoTransactions
}
