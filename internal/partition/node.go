package partition

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/metrics"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/partition/event"
	pgenesis "github.com/alphabill-org/alphabill/internal/partition/genesis"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/txbuffer"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	log "github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	initializing status = iota
	normal
	recovering
)

const t1TimerName = "t1"

// Key 0 is used for proposal, that way it is still possible to reverse iterate the DB
// and use 4 byte key, make it incompatible with block number
const proposalKey = uint32(0)
const ledgerReplicationTimeout = 1500 * time.Millisecond

var (
	ErrNodeDoesNotHaveLatestBlock = errors.New("recovery needed, node does not have the latest block")

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
		lastStoredBlock             *block.Block
		proposedTransactions        []txsystem.GenericTransaction
		pendingBlockProposal        *block.GenericPendingBlockProposal
		timers                      *timer.Timers
		leaderSelector              LeaderSelector
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  keyvaluedb.KeyValueDB
		txBuffer                    *txbuffer.TxBuffer
		ctx                         context.Context
		ctxCancel                   context.CancelFunc
		network                     Net
		txCancel                    context.CancelFunc
		txWaitGroup                 *sync.WaitGroup
		txCh                        chan txsystem.GenericTransaction
		eventCh                     chan event.Event
		eventChCancel               chan bool
		lastRootMsgTime             time.Time
		lastLedgerReqTime           time.Time
		eventHandler                event.Handler
		recoveryLastProp            *blockproposal.BlockProposal
		ticker                      *time.Ticker
	}

	status int
)

// New creates a new instance of the partition node. All parameters expect the nodeOptions are required. Functions
// implementing the NodeOption interface can be used to override default configuration values:
//
//	  n, err := New(
//	 	peer,
//			signer,
//			txSystem,
//			genesis,
//			net,
//			WithContext(context.Background()),
//			WithTxValidator(myTxValidator)),
//			WithUnicityCertificateValidator(ucValidator),
//			WithBlockProposalValidator(blockProposalValidator),
//			WithLeaderSelector(leaderSelector),
//			WithBlockStore(blockStore),
//			WithT1Timeout(250*time.Millisecond),
//	  )
//
// The following restrictions apply to the inputs:
//   - the network peer and signer must use the same keys that were used to generate node genesis file;
//   - the state of the transaction system must be equal to the state that was used to generate genesis file.
func New(
	peer *network.Peer, // P2P peer for the node
	signer crypto.Signer, // used to sign block proposals and block certification requests
	txSystem txsystem.TransactionSystem, // used transaction system
	genesis *genesis.PartitionGenesis, // partition genesis file, created by root chain.
	net Net, // network layer of the node
	nodeOptions ...NodeOption, // additional optional configuration parameters
) (*Node, error) {
	// load and validate node configuration
	conf, err := loadAndValidateConfiguration(peer, signer, genesis, txSystem, net, nodeOptions...)
	if err != nil {
		return nil, fmt.Errorf("node error, %w", err)
	}

	log.SetContext(log.KeyNodeID, conf.peer.ID().String())

	n := &Node{
		status:                      initializing,
		configuration:               conf,
		transactionSystem:           txSystem,
		timers:                      timer.NewTimers(),
		leaderSelector:              conf.leaderSelector,
		txValidator:                 conf.txValidator,
		unicityCertificateValidator: conf.unicityCertificateValidator,
		blockProposalValidator:      conf.blockProposalValidator,
		blockStore:                  conf.blockStore,
		txBuffer:                    conf.txBuffer,
		eventHandler:                conf.eventHandler,
		network:                     net,
		txWaitGroup:                 &sync.WaitGroup{},
		lastRootMsgTime:             time.Time{},
		lastLedgerReqTime:           time.Time{},
		ticker:                      time.NewTicker(time.Second),
	}
	n.ctx, n.ctxCancel = context.WithCancel(conf.context)

	n.txCh = make(chan txsystem.GenericTransaction, conf.txBuffer.Capacity())
	if n.eventHandler != nil {
		n.eventCh = make(chan event.Event, conf.eventChCapacity)
		n.eventChCancel = make(chan bool)
		go n.eventHandlerLoop()
	}

	if err = initState(n); err != nil {
		return nil, fmt.Errorf("node init failed, %w", err)
	}
	go n.loop()
	// subscribe to unicity certificates
	n.sendHandshake()
	return n, nil
}

func (n *Node) getCurrentRound() uint64 {
	return n.luc.GetRoundNumber() + 1
}

func (n *Node) sendHandshake() {
	logger.Trace("Sending handshake to root chain")
	if err := n.network.Send(network.OutputMessage{
		Protocol: network.ProtocolHandshake,
		Message: &handshake.Handshake{
			SystemIdentifier: n.configuration.GetSystemIdentifier(),
			NodeIdentifier:   n.leaderSelector.SelfID().String(),
		},
	}, []peer.ID{n.configuration.rootChainID}); err != nil {
		logger.Error("error sending handshake", err)
	}
}

func initState(n *Node) error {
	defer trackExecutionTime(time.Now(), "Restore node state")
	// get genesis block from the genesis
	genesisBlock := n.configuration.genesisBlock()
	if n.blockStore.Empty() {
		logger.Info("State initialised from genesis")
		if err := n.blockStore.Write(util.Uint64ToBytes(pgenesis.PartitionRoundNumber), genesisBlock); err != nil {
			return fmt.Errorf("init failed to persist genesis block, %w", err)
		}
		n.transactionSystem.Commit() // commit everything from the genesis
		// set luc to last known uc
		n.luc = genesisBlock.UnicityCertificate
		n.lastStoredBlock = genesisBlock
		return nil
	}
	// restore state from db
	prevBlock := genesisBlock
	// get next block from genesis block
	dbIt := n.blockStore.Find(util.Uint64ToBytes(pgenesis.PartitionRoundNumber + 1))
	defer func() {
		if err := dbIt.Close(); err != nil {
			logger.Warning("Unexpected DB iterator error %v", err)
		}
	}()
	for ; dbIt.Valid(); dbIt.Next() {
		var bl block.Block
		roundNo := util.BytesToUint64(dbIt.Key())
		if err := dbIt.Value(&bl); err != nil {
			return fmt.Errorf("failed to read block %v from db, %w", roundNo, err)
		}
		if !bytes.Equal(prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.PreviousBlockHash) {
			return fmt.Errorf("invalid blockchain (previous block %v hash='%X', current block %v backlink='%X')", prevBlock.UnicityCertificate.InputRecord.RoundNumber, prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.UnicityCertificate.InputRecord.RoundNumber, bl.PreviousBlockHash)
		}
		state, err := n.applyBlockTransactions(bl.GetRoundNumber(), bl.Transactions)
		if err != nil {
			return fmt.Errorf("block %v apply transactions failed, %w", roundNo, err)
		}
		if err = verifyTxSystemState(state, bl.UnicityCertificate.InputRecord); err != nil {
			return fmt.Errorf("block %v, state mismatch, %w", roundNo, err)
		}
		// commit changes
		n.transactionSystem.Commit()
		prevBlock = &bl
	}
	logger.Info("State initialised from persistent store up to block %v", prevBlock.UnicityCertificate.InputRecord.RoundNumber)
	// update luc to last known uc
	n.luc = prevBlock.UnicityCertificate
	n.lastStoredBlock = prevBlock
	n.restoreBlockProposal(prevBlock)
	return nil
}

func verifyTxSystemState(state txsystem.State, ucIR *certificates.InputRecord) error {
	if ucIR == nil {
		return errors.New("unicity certificate input record is nil")
	}
	if !bytes.Equal(ucIR.Hash, state.Root()) {
		return fmt.Errorf("tx system state does not match unicity certificate")
	} else if !bytes.Equal(ucIR.SummaryValue, state.Summary()) {
		return fmt.Errorf("tx system summary value %X not equal to unicity certificte value %X", ucIR.SummaryValue, state.Summary())
	}
	return nil
}

func (n *Node) applyBlockTransactions(round uint64, txs []*txsystem.Transaction) (txsystem.State, error) {
	n.transactionSystem.BeginBlock(round)
	for _, tx := range txs {
		gtx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			return nil, fmt.Errorf("tx '%v' conversion error, %w", tx, err)
		}
		if err = n.validateAndExecuteTx(gtx, round); err != nil {
			return nil, fmt.Errorf("tx '%v' execution error, %w", tx, err)
		}
	}
	return n.transactionSystem.EndBlock()
}

func (n *Node) restoreBlockProposal(prevBlock *block.Block) {
	var pr block.PendingBlockProposal
	found, err := n.blockStore.Read(util.Uint32ToBytes(proposalKey), &pr)
	if err != nil {
		logger.Error("Error fetching block proposal: %s", err)
		return
	}
	if !found {
		logger.Debug("No pending block proposal stored")
		return
	}
	// make sure proposal extends the previous state
	if !bytes.Equal(prevBlock.UnicityCertificate.InputRecord.Hash, pr.PrevHash) {
		logger.Debug("Stored block proposal does not extend previous state, stale proposal")
		return
	}
	proposal, err := pr.ToGeneric(n.transactionSystem)
	if err != nil {
		logger.Warning("Error restoring block proposal: %v", err)
	}
	// apply stored proposal to current state
	logger.Debug("Stored block proposal extends the previous state")
	roundNo := prevBlock.UnicityCertificate.InputRecord.RoundNumber + 1
	state, err := n.applyBlockTransactions(roundNo, pr.Transactions)
	if err != nil {
		logger.Warning("Block proposal recovery failed, %w", err)
		n.revertState()
		return
	}
	if !bytes.Equal(proposal.StateHash, state.Root()) {
		logger.Warning("Block proposal transaction failed, state hash mismatch", err)
		n.revertState()
		return
	}
	// wait for UC to certify the block proposal
	n.pendingBlockProposal = proposal
}

// Close shuts down the Node component.
func (n *Node) Close() {
	logger.Info("Shutting down node '%v'", n.configuration.peer.ID())
	n.ctxCancel()
	n.ticker.Stop()
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
				if err := n.process(tx, n.getCurrentRound()); err != nil {
					logger.Warning("Processing tx failed, %w", err)
				}
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
				util.WriteTraceJsonLog(logger, "Unicity Certificate:", uc)
				n.lastRootMsgTime = time.Now()
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
		case nt, ok := <-n.timers.C:
			if !ok {
				logger.Warning("Timers channel closed, exiting main loop")
				return
			}
			if nt == nil {
				continue
			}
			n.handleT1TimeoutEvent()
		case _, ok := <-n.ticker.C:
			if !ok {
				logger.Warning("Ticker channel closed, exiting main loop")
				return
			}
			n.handleMonitoring()
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
		return fmt.Errorf("unsupported type: %T", m.Message)
	}
	genTx, err := n.transactionSystem.ConvertTx(tx)
	if err != nil {
		return err
	}
	return n.txBuffer.Add(genTx)
}

func (n *Node) handleOrForwardTransaction(tx txsystem.GenericTransaction) bool {
	rn := n.luc.InputRecord.RoundNumber + 1
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
	err := n.network.Send(
		network.OutputMessage{
			Protocol: network.ProtocolInputForward,
			Message:  tx.ToProtoBuf(),
		},
		[]peer.ID{leader},
	)
	// TODO unreported error?
	return err == nil
}

func (n *Node) process(tx txsystem.GenericTransaction, round uint64) error {
	defer trackExecutionTime(time.Now(), "Processing transaction")
	if err := n.validateAndExecuteTx(tx, round); err != nil {
		n.sendEvent(event.TransactionFailed, tx)
		return fmt.Errorf("tx '%v' execution failed, %w", tx, err)
	}
	n.proposedTransactions = append(n.proposedTransactions, tx)
	n.sendEvent(event.TransactionProcessed, tx)
	logger.Debug("Transaction processed by node %v. Proposal size: %v", n.configuration.peer.ID(), len(n.proposedTransactions))
	return nil
}

func (n *Node) validateAndExecuteTx(tx txsystem.GenericTransaction, round uint64) error {
	if err := n.txValidator.Validate(tx, round); err != nil {
		logger.Warning("Transaction '%v' is invalid: %v", tx, err)
		return fmt.Errorf("invalid, %w", err)
	}
	if err := n.transactionSystem.Execute(tx); err != nil {
		logger.Warning("TxSystem was unable to process transaction '%v': %v", tx, err)
		return fmt.Errorf("execute error, %w", err)
	}
	return nil
}

// handleBlockProposal processes a block proposals. Performs the following steps:
//  1. Block proposal as a whole is validated:
//     * It must have valid signature, correct transaction system ID, valid UC;
//     * the UC must be not older than the latest known by current node;
//     * Sender must be the leader for the round started by included UC.
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
		// but remember last block proposal received
		n.recoveryLastProp = prop
		return nil
	}
	defer trackExecutionTime(time.Now(), "Handling BlockProposal")
	if prop == nil {
		return blockproposal.ErrBlockProposalIsNil
	}
	nodeSignatureVerifier, err := n.configuration.GetSigningPublicKey(prop.NodeIdentifier)
	if err != nil {
		return fmt.Errorf("unknown node id, %w", err)
	}
	if err = n.blockProposalValidator.Validate(prop, nodeSignatureVerifier); err != nil {
		return fmt.Errorf("block proposal validation failed, %w", err)
	}
	logger.Debug("Handling block proposal, its UC IR Hash %X, Block hash %X", prop.UnicityCertificate.InputRecord.Hash, prop.UnicityCertificate.InputRecord.BlockHash)
	// verify proposal unicity certificate, must not be older than latest seen received by the node
	uc := prop.UnicityCertificate
	// UC must be newer than the last one seen
	if uc.InputRecord.RoundNumber < n.luc.InputRecord.RoundNumber {
		return fmt.Errorf("received UC is older, uc round %v, luc round %v",
			uc.InputRecord.RoundNumber, n.luc.InputRecord.RoundNumber)
	}
	expectedLeader := n.leaderSelector.LeaderFromUnicitySeal(uc)
	if expectedLeader == UnknownLeader || prop.NodeIdentifier != expectedLeader.String() {
		return fmt.Errorf("invalid node identifier. leader from UC: %v, request leader: %v", expectedLeader, prop.NodeIdentifier)
	}
	if uc.InputRecord.RoundNumber > n.luc.InputRecord.RoundNumber {
		// either the other node received it faster from root or there must be some issue with root communication?
		logger.Debug("Received newer UC round nr %v via block proposal, LUC round %v", uc.InputRecord.RoundNumber, n.luc.InputRecord.RoundNumber)
		// just to be sure, subscribe to root chain again, this may result in a duplicate UC received
		n.sendHandshake()
		if err = n.handleUnicityCertificate(uc); err != nil {
			return fmt.Errorf("block proposal unicity cerrificate hanlding faild, %w", err)
		}
	}
	prevHash := uc.InputRecord.Hash
	txState, err := n.transactionSystem.State()
	if err != nil {
		return fmt.Errorf("tx system state error, %w", err)
	}
	// check previous state matches before processing transactions
	if !bytes.Equal(prevHash, txState.Root()) {
		return fmt.Errorf("tx system start state mismatch error, expected: %X, got: %X", txState.Root(), prevHash)
	}
	n.transactionSystem.BeginBlock(n.luc.InputRecord.RoundNumber + 1)
	for _, tx := range prop.Transactions {
		genTx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			logger.Warning("transaction is invalid %v", err)
			return err
		}
		if err = n.process(genTx, n.getCurrentRound()); err != nil {
			return fmt.Errorf("transaction error %w", err)
		}
	}
	if err = n.sendCertificationRequest(); err != nil {
		return fmt.Errorf("certification request send failed, %w", err)
	}
	return nil
}

func (n *Node) updateLUC(uc *certificates.UnicityCertificate) {
	if n.luc != nil && uc.InputRecord.RoundNumber <= n.luc.InputRecord.RoundNumber {
		return
	}
	n.luc = uc
	logger.Debug("Updated LUC, round: %v", n.luc.InputRecord.RoundNumber)
	n.sendEvent(event.LatestUnicityCertificateUpdated, uc)
}

func (n *Node) startNewRound(uc *certificates.UnicityCertificate) {
	n.updateLUC(uc)
	if n.status == recovering {
		logger.Info("Node is recovered until block %v", uc.InputRecord.RoundNumber)
		n.sendEvent(event.RecoveryFinished, uc.InputRecord.RoundNumber)
	}
	n.status = normal
	newRoundNr := uc.InputRecord.RoundNumber + 1
	n.transactionSystem.BeginBlock(newRoundNr)
	n.proposedTransactions = []txsystem.GenericTransaction{}
	n.pendingBlockProposal = nil
	// not a fatal issue, but log anyway
	if err := n.blockStore.Delete(util.Uint32ToBytes(proposalKey)); err != nil {
		logger.Debug("DB delete failed, %w", err)
	}
	n.leaderSelector.UpdateLeader(uc)
	n.startHandleOrForwardTransactions()
	n.timers.Start(t1TimerName, n.configuration.t1Timeout)
	n.sendEvent(event.NewRoundStarted, newRoundNr)
}

func (n *Node) startRecovery(uc *certificates.UnicityCertificate) {
	// always update last UC seen, this is needed to evaluate if node has recovered and is up-to-date
	n.updateLUC(uc)
	if n.status == recovering {
		// already recovering, but if uc is newer than luc, let's update luc
		logger.Debug("Recovery already in progress, recovering to %v", n.luc.InputRecord.RoundNumber)
		return
	}
	// starting recovery
	n.revertState()
	n.status = recovering
	n.stopForwardingOrHandlingTransactions()
	logger.Debug("Entering recovery state, recover node up to %v", n.luc.InputRecord.RoundNumber)
	fromBlockNr := n.lastStoredBlock.UnicityCertificate.InputRecord.RoundNumber + 1
	n.sendEvent(event.RecoveryStarted, fromBlockNr)
	n.sendLedgerReplicationRequest(fromBlockNr)
}

// handleUnicityCertificate processes the Unicity Certificate and finalizes a block. Performs the following steps:
//  1. Given UC is validated cryptographically -> checked before this method is called by unicityCertificateValidator
//  2. Given UC has correct system identifier -> checked before this method is called by unicityCertificateValidator
//  3. TODO: sanity check timestamp
//  4. Given UC is checked for equivocation (for more details see certificates.CheckNonEquivocatingCertificates)
//  5. On unexpected case where there is no pending block proposal, recovery is initiated, unless the state is already
//     up-to-date with the given UC.
//  6. Alternatively, if UC certifies the pending block proposal then block is finalized.
//  7. Alternatively, if UC certifies repeat IR (‘repeat UC’) then
//     state is rolled back to previous state.
//  8. Alternatively, recovery is initiated, after rollback. Note that recovery may end up with
//     newer last known UC than the one being processed.
//  8. New round is started.
//
// See algorithm 5 "Processing a received Unicity Certificate" in Yellowpaper for more details
func (n *Node) handleUnicityCertificate(uc *certificates.UnicityCertificate) error {
	defer trackExecutionTime(time.Now(), "Handling unicity certificate")
	if uc == nil {
		return fmt.Errorf("unicity certificate is nil")
	}
	// UC is validated cryptographically
	if err := n.unicityCertificateValidator.Validate(uc); err != nil {
		n.sendEvent(event.Error, err)
		return fmt.Errorf("certificate invalid, %w", err)
	}
	// validation must make sure all mandatory fields are present and UC is cryptographically sound
	// from this point fields can be logged, that must not be nil can be logged
	logger.Debug("Received Unicity Certificate:\nH:\t%X\nH':\t%X\nHb:\t%X", uc.InputRecord.Hash, uc.InputRecord.PreviousHash, uc.InputRecord.BlockHash)
	logger.Debug("LUC:\nH:\t%X\nH':\t%X\nHb:\t%X", n.luc.InputRecord.Hash, n.luc.InputRecord.PreviousHash, n.luc.InputRecord.BlockHash)
	// ignore duplicates
	if bytes.Equal(n.luc.InputRecord.Bytes(), uc.InputRecord.Bytes()) {
		if n.status == initializing {
			// first UC seen and as and node is already up-to-date
			// either starting from genesis or a very quick restart
			n.startNewRound(uc)
		}
		return nil
	}
	// check for equivocation
	if err := certificates.CheckNonEquivocatingCertificates(n.luc, uc); err != nil {
		// this is not normal, log all info
		logger.Warning("Round %v UC error, %v", uc.InputRecord.RoundNumber, err)
		logger.Warning("LUC:\n%s", util.EncodeToJsonHelper(n.luc))
		logger.Warning("equivocating UC:\n%s", util.EncodeToJsonHelper(uc))
		return fmt.Errorf("equivocating certificate, %w", err)
	}
	// If there is no pending block proposal i.e. no certification request has been sent by the node
	// - leader was down and did not make a block proposal?
	// - node did not receive a block proposal because it was down, it was not sent or there were network issues
	if n.pendingBlockProposal == nil {
		// Start recovery unless the state is already up-to-date with UC.
		state, err := n.transactionSystem.State()
		if err != nil {
			logger.Warning("Recovery needed, failed to get tx system state, %v", err)
			n.startRecovery(uc)
			return fmt.Errorf("recovery needed, transaction system state error, %w", err)
		}
		// if state hash does not match - start recovery
		if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			logger.Warning("Recovery needed, UC IR hash not equal to state's hash: '%X' vs '%X'", uc.InputRecord.Hash, state.Root())
			n.startRecovery(uc)
			return ErrNodeDoesNotHaveLatestBlock
		}
		logger.Debug("No pending block proposal, UC IR hash is equal to State hash, so are block hashes")
		n.startNewRound(uc)
		return nil
	}
	// Check pending block proposal
	bl, blockHash, err := n.proposalHash(n.pendingBlockProposal.Transactions, uc)
	logger.Debug("Pending proposal: \nH:\t%X\nH':\t%X\nHb:\t%X\nround:\t%v",
		n.pendingBlockProposal.StateHash, n.pendingBlockProposal.PrevHash, blockHash, n.pendingBlockProposal.RoundNumber)
	if err != nil {
		logger.Warning("Recovery needed, block proposal hash calculation error, %v", err)
		n.startRecovery(uc)
		return fmt.Errorf("recovery needed, block proposal hash calculation failed, %w", err)
	}
	if bytes.Equal(uc.InputRecord.Hash, n.pendingBlockProposal.StateHash) {
		// UC certifies pending block proposal
		if err = n.finalizeBlock(bl); err != nil {
			logger.Warning("Recovery needed, block finalize failed, %v", err)
			n.startRecovery(uc)
			return fmt.Errorf("block %v finalize failed, %w", bl.UnicityCertificate.InputRecord.RoundNumber, err)
		}
		n.startNewRound(uc)
		return nil
	}
	// repeat UC
	if bytes.Equal(uc.InputRecord.Hash, n.pendingBlockProposal.PrevHash) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		logger.Warning("Reverting state tree on repeat certificate. UC IR hash: %X, proposal hash %X", uc.InputRecord.Hash, n.pendingBlockProposal.PrevHash)
		n.revertState()
		n.startNewRound(uc)
		return nil
	}
	// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
	// revertState is called from startRecovery()
	logger.Warning("Recovery needed, either proposal state hash or block hash is different")
	n.startRecovery(uc)
	return ErrNodeDoesNotHaveLatestBlock
}

func (n *Node) revertState() {
	logger.Warning("Reverting state")
	n.sendEvent(event.StateReverted, nil)
	n.transactionSystem.Revert()
}

func (n *Node) proposalHash(transactions []txsystem.GenericTransaction, uc *certificates.UnicityCertificate) (*block.Block, []byte, error) {
	b := &block.GenericBlock{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		// latest non-empty block
		PreviousBlockHash:  n.lastStoredBlock.UnicityCertificate.InputRecord.BlockHash,
		Transactions:       transactions,
		UnicityCertificate: uc,
	}
	blockHash, err := b.Hash(n.configuration.hashAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("block hash calculation failed, %w", err)
	}
	return b.ToProtobuf(), blockHash, nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(b *block.Block) error {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("Block %v finalization", b.UnicityCertificate.InputRecord.RoundNumber))
	// if empty block then ignore this block
	if len(b.Transactions) == 0 {
		n.sendEvent(event.BlockFinalized, b)
		return nil
	}
	roundNoInBytes := util.Uint64ToBytes(b.UnicityCertificate.InputRecord.RoundNumber)
	if err := n.blockStore.Write(roundNoInBytes, b); err != nil {
		return fmt.Errorf("db write failed, %w", err)
	}
	// cache last stored non-empty block, but only if store succeeds
	// NB! only cache and commit if persist is successful
	n.lastStoredBlock = b
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
	logger.Info("Handling T1 timeout")
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

// handleMonitoring - monitors root communication, if for no UC is
// received for a long time then try and request one from root
func (n *Node) handleMonitoring() {
	// check if we have not heard from root validator for a long time
	if time.Since(n.lastRootMsgTime) > 2*n.configuration.GetT2Timeout() {
		// subscribe again
		n.sendHandshake()
	}
	// handle ledger replication timeout - no response from node is received
	if n.status == recovering && time.Since(n.lastLedgerReqTime) > ledgerReplicationTimeout {
		logger.Warning("Ledger replication timeout, repeat request")
		n.sendLedgerReplicationRequest(n.lastStoredBlock.GetRoundNumber() + 1)
	}
}

func (n *Node) sendLedgerReplicationResponse(msg *replication.LedgerReplicationResponse, toId string) error {
	recoveringNodeID, err := peer.Decode(toId)
	if err != nil {
		return fmt.Errorf("failed to send, peer id %s decode failed, %w", toId, err)
	}

	if err = n.network.Send(network.OutputMessage{
		Protocol: network.ProtocolLedgerReplicationResp,
		Message:  msg,
	}, []peer.ID{recoveringNodeID}); err != nil {
		return fmt.Errorf("replication response %s send failed, %v", msg.Pretty(), err)
	}
	n.sendEvent(event.ReplicationResponseSent, msg)
	return nil
}

func (n *Node) handleLedgerReplicationRequest(lr *replication.LedgerReplicationRequest) error {
	util.WriteTraceJsonLog(logger, "Ledger replication request received:", lr)
	if err := lr.IsValid(); err != nil {
		// for now do not respond to obviously invalid requests
		return fmt.Errorf("invalid request, %w", err)
	}
	// check if the node is known
	if _, err := n.configuration.GetSigningPublicKey(lr.NodeIdentifier); err != nil {
		return fmt.Errorf("unknown node, %w", err)
	}
	if !bytes.Equal(lr.SystemIdentifier, n.configuration.GetSystemIdentifier()) {
		resp := &replication.LedgerReplicationResponse{
			Status:  replication.LedgerReplicationResponse_UNKNOWN_SYSTEM_IDENTIFIER,
			Message: fmt.Sprintf("Unknown system identifier: %X", lr.GetSystemIdentifier()),
		}
		return n.sendLedgerReplicationResponse(resp, lr.NodeIdentifier)
	}
	maxBlock := n.lastStoredBlock.GetRoundNumber()
	startBlock := lr.BeginBlockNumber
	// the node is behind and does not have the needed data
	if maxBlock < startBlock {
		resp := &replication.LedgerReplicationResponse{
			Status:  replication.LedgerReplicationResponse_BLOCKS_NOT_FOUND,
			Message: fmt.Sprintf("Node does not have block: %v, latest block: %v", startBlock, maxBlock),
		}
		return n.sendLedgerReplicationResponse(resp, lr.NodeIdentifier)
	}
	logger.Debug("Preparing replication response from block %v", startBlock)
	go func() {
		blocks := make([]*block.Block, 0)
		countTx := uint32(0)
		blockCnt := uint64(0)
		dbIt := n.blockStore.Find(util.Uint64ToBytes(startBlock))
		defer func() {
			if err := dbIt.Close(); err != nil {
				logger.Warning("Unexpected DB iterator error %v", err)
			}
		}()
		for ; dbIt.Valid(); dbIt.Next() {
			var bl block.Block
			roundNo := util.BytesToUint64(dbIt.Key())
			if err := dbIt.Value(&bl); err != nil {
				logger.Warning("Ledger replication reply incomplete, block %v read failed %v", roundNo, err)
				break
			}
			blocks = append(blocks, &bl)
			blockCnt++
			countTx += uint32(len(bl.Transactions))
			if countTx >= n.configuration.replicationConfig.maxTx ||
				blockCnt >= n.configuration.replicationConfig.maxBlocks {
				break
			}
		}
		resp := &replication.LedgerReplicationResponse{
			Status: replication.LedgerReplicationResponse_OK,
			Blocks: blocks,
		}
		if err := n.sendLedgerReplicationResponse(resp, lr.NodeIdentifier); err != nil {
			logger.Warning("Problem sending ledger replication response, %s: %s", resp.Pretty(), err)
		}
	}()
	return nil
}

// handleLedgerReplicationResponse handles ledger replication responses from other partition nodes.
// This method is an approximation of YellowPaper algorithm 10 "Partition Node Recovery" (synchronous algorithm)
func (n *Node) handleLedgerReplicationResponse(lr *replication.LedgerReplicationResponse) error {
	if err := lr.IsValid(); err != nil {
		return fmt.Errorf("invalid ledger replication response, %w", err)
	}
	if n.status != recovering {
		logger.Debug("Stale Ledger Replication response, node is not recovering: %s", lr.Pretty())
		return nil
	}
	logger.Debug("Ledger replication response received: %s, ", lr.Pretty())
	if lr.Status != replication.LedgerReplicationResponse_OK {
		recoverFrom := n.lastStoredBlock.GetRoundNumber() + 1
		logger.Debug("Resending replication request starting with round %v", recoverFrom)
		n.sendLedgerReplicationRequest(recoverFrom)
		return fmt.Errorf("received error response, status=%s, message='%s'", lr.Status.String(), lr.Message)
	}
	var err error
	for _, b := range lr.Blocks {
		if err = b.IsValid(n.unicityCertificateValidator); err != nil {
			// sends invalid blocks, do not trust the response and try again
			err = fmt.Errorf("ledger replication response contains invalid block, %w", err)
			break
		}
		// it could be that we receive blocks from earlier time or later time, make sure to extend from what is missing
		roundNo := b.GetRoundNumber()
		// skip earlier blocks
		if roundNo <= n.lastStoredBlock.GetRoundNumber() {
			logger.Debug("Node already has this block %v, skipping block %v", n.lastStoredBlock.GetRoundNumber(), roundNo)
			continue
		}
		logger.Debug("Recovering block from round %v", roundNo)
		// make sure it extends current state
		var state txsystem.State
		state, err = n.transactionSystem.State()
		if err != nil {
			err = fmt.Errorf("error reading current state, %w", err)
			break
		}
		if !bytes.Equal(b.UnicityCertificate.InputRecord.PreviousHash, state.Root()) {
			err = fmt.Errorf("received block does not extend current state")
			break
		}
		state, err = n.applyBlockTransactions(b.GetRoundNumber(), b.Transactions)
		if err != nil {
			return fmt.Errorf("block %v apply transactions failed, %w", roundNo, err)
		}
		if err = verifyTxSystemState(state, b.UnicityCertificate.InputRecord); err != nil {
			return fmt.Errorf("block %v, state mismatch, %w", roundNo, err)
		}
		// update DB and last block
		if err = n.finalizeBlock(b); err != nil {
			err = fmt.Errorf("block %v persist failed, %w", b.UnicityCertificate.InputRecord.RoundNumber, err)
			break
		}
	}
	latestStoredBlockUc := n.lastStoredBlock.UnicityCertificate
	// log problems
	if err != nil {
		// Revert any transactions that were applied
		n.revertState()
		// ask the for the failed block again, what else can we do?
		n.sendLedgerReplicationRequest(latestStoredBlockUc.GetRoundNumber() + 1)
		return err
	}
	// check if recovery is complete
	logger.Debug("Checking if recovery is complete, last block is from round: %v", latestStoredBlockUc.GetRoundNumber())
	// every non-empty block is guaranteed to change state hash, meaning if the state hash is equal to luc state hash
	// then recovery is complete
	if !bytes.Equal(latestStoredBlockUc.InputRecord.Hash, n.luc.InputRecord.Hash) {
		logger.Debug("Not fully recovered yet, latest block's UC root round %v vs LUC's root round %v", latestStoredBlockUc.GetRoundNumber(), n.luc.GetRoundNumber())
		n.sendLedgerReplicationRequest(latestStoredBlockUc.GetRoundNumber() + 1)
		return nil
	}
	// node should be recovered now, stop recovery and change state to normal
	n.startNewRound(n.luc)
	// try to apply the last received block proposal received during recovery, it may fail if the block was finalized and
	// is in fact the last block received
	if n.recoveryLastProp != nil {
		// try to apply it to the latest state, may fail
		if err = n.handleBlockProposal(n.recoveryLastProp); err != nil {
			logger.Debug("Recovery completed, failed to apply last received block proposal(stale?), %v", err)
		}
		n.recoveryLastProp = nil
	}
	return nil
}

func (n *Node) sendLedgerReplicationRequest(startingBlockNr uint64) {
	req := &replication.LedgerReplicationRequest{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		NodeIdentifier:   n.leaderSelector.SelfID().String(),
		BeginBlockNumber: startingBlockNr,
	}
	util.WriteTraceJsonLog(logger, "Ledger replication request:", req)
	peers := n.configuration.peer.Validators()
	if len(peers) == 0 {
		logger.Warning("Error sending ledger replication request, no peers")
	}
	var err error
	requestSent := false
	// send Ledger Replication request to a first alive randomly chosen node
	for _, p := range util.ShuffleSliceCopy(peers) {
		logger.Debug("Sending ledger replication request to peer '%v'", p)
		err = n.network.Send(network.OutputMessage{
			Protocol: network.ProtocolLedgerReplicationReq,
			Message:  req,
		}, []peer.ID{p})
		// break loop on successful send, otherwise try again but different node, until all either
		// able to send or all attempts have failed
		if err == nil {
			requestSent = true
			break
		}
		// log every failed attempt?
		logger.Debug("Error sending ledger replication request: %s", err)
	}
	if requestSent == false {
		logger.Warning("Error sending ledger replication request, all peers down")
		return
	}
	// remember last request sent for timeout handling - if no response is received
	n.lastLedgerReqTime = time.Now()
}

func (n *Node) sendBlockProposal() error {
	defer trackExecutionTime(time.Now(), "Sending BlockProposal")
	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.leaderSelector.SelfID()
	prop := &blockproposal.BlockProposal{
		SystemIdentifier:   systemIdentifier,
		NodeIdentifier:     nodeId.String(),
		UnicityCertificate: n.luc,
		Transactions:       block.GenericTxsToProtobuf(n.proposedTransactions),
	}
	util.WriteTraceJsonLog(logger, "BlockProposal created:", prop)
	if err := prop.Sign(n.configuration.hashAlgorithm, n.configuration.signer); err != nil {
		return fmt.Errorf("block proposal sign failed, %w", err)
	}
	return n.network.Send(network.OutputMessage{
		Protocol: network.ProtocolBlockProposal,
		Message:  prop,
	}, n.configuration.peer.Validators())
}

func (n *Node) persistBlockProposal(pr *block.GenericPendingBlockProposal) error {
	proposal := pr.ToProtobuf()
	if err := n.blockStore.Write(util.Uint32ToBytes(proposalKey), proposal); err != nil {
		return fmt.Errorf("persist error, %w", err)
	}
	return nil
}

func (n *Node) sendCertificationRequest() error {
	defer trackExecutionTime(time.Now(), "Sending CertificationRequest")
	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.leaderSelector.SelfID()
	prevStateHash := n.luc.InputRecord.Hash
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return fmt.Errorf("tx system failed to end block, %w", err)
	}
	stateHash := state.Root()
	summary := state.Summary()

	pendingProposal := &block.GenericPendingBlockProposal{
		RoundNumber:  n.luc.InputRecord.RoundNumber + 1,
		PrevHash:     prevStateHash,
		StateHash:    stateHash,
		Transactions: n.proposedTransactions,
	}
	if err = n.persistBlockProposal(pendingProposal); err != nil {
		logger.Error("failed to store proposal, %v", err)
		return fmt.Errorf("failed to store pending block proposal, %w", err)
	}
	n.pendingBlockProposal = pendingProposal

	latestBlockHash := n.lastStoredBlock.UnicityCertificate.InputRecord.BlockHash
	blockHash, err := n.hashProposedBlock(latestBlockHash)
	if err != nil {
		return fmt.Errorf("block hash calculation failed, %w", err)
	}
	n.proposedTransactions = []txsystem.GenericTransaction{}

	req := &certification.BlockCertificationRequest{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId.String(),
		InputRecord: &certificates.InputRecord{
			PreviousHash: pendingProposal.PrevHash,
			Hash:         pendingProposal.StateHash,
			BlockHash:    blockHash,
			SummaryValue: summary,
			// latestBlock is the latest non-empty block,
			// latest UC might have certified an empty block and has the latest round number
			RoundNumber: pendingProposal.RoundNumber,
		},
	}
	if err = req.Sign(n.configuration.signer); err != nil {
		return fmt.Errorf("failed to sign certification req, %w", err)
	}
	logger.Info("Round %v sending block certification request to root chain, IR hash %X, Block Hash %X",
		pendingProposal.RoundNumber, stateHash, blockHash)
	util.WriteTraceJsonLog(logger, "Block Certification req:", req)

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
	rn := n.luc.InputRecord.RoundNumber + 1
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
	// find and return closest match from db
	if blockNr == 0 {
		return nil, fmt.Errorf("block number 0 does not exist")
	}
	var bl block.Block
	found, err := n.blockStore.Read(util.Uint64ToBytes(blockNr), &bl)
	if err != nil {
		return nil, fmt.Errorf("failed to read block from round %v from db, %w", blockNr, err)
	}
	if !found {
		// empty block
		return nil, nil
	}
	return &bl, nil
}

func (n *Node) GetLatestBlock() (*block.Block, error) {
	dbIt := n.blockStore.Last()
	defer func() {
		if err := dbIt.Close(); err != nil {
			logger.Warning("Unexpected DB iterator error %v", err)
		}
	}()
	var bl block.Block
	roundNo := util.BytesToUint64(dbIt.Key())
	if err := dbIt.Value(&bl); err != nil {
		return nil, fmt.Errorf("failed to read block %v from db, %w", roundNo, err)
	}
	return &bl, nil
}

func (n *Node) GetLatestRoundNumber() (uint64, error) {
	return n.luc.GetRoundNumber(), nil
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
	go func() {
		defer n.txWaitGroup.Done()
		n.txBuffer.Process(txCtx, n.handleOrForwardTransaction)
	}()
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
	switch r := event.(type) {
	case T:
		return true, r
	}
	return false, result
}

func trackExecutionTime(start time.Time, name string) {
	logger.Debug("%s took %s", name, time.Since(start))
}
