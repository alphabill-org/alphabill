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
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/partition/store"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/txbuffer"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	idle status = iota
	recovering
)

const (
	EventTypeError EventType = iota
	EventTypeTransactionProcessed
	EventTypeNewRoundStarted
	EventTypeUnicityCertificateHandled
	EventTypeBlockFinalized
	EventTypeRecoveryStarted
	EventTypeRecoveryFinished
)

const t1TimerName = "t1"

var (
	ErrNodeDoesNotHaveLatestBlock = errors.New("node does not have the latest block")
	ErrStateReverted              = errors.New("state reverted")
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
		proposal                    []txsystem.GenericTransaction
		pr                          *block.PendingBlockProposal
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
		txCtx                       context.Context
		txCancel                    context.CancelFunc
		txWaitGroup                 *sync.WaitGroup
		txCh                        chan txsystem.GenericTransaction
		eventCh                     chan Event
		eventChCancel               chan bool
		eventHandler                EventHandler
	}

	Event struct {
		EventType EventType
		Content   any
	}

	EventType int

	EventHandler func(e Event)

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
		n.eventCh = make(chan Event, conf.eventChCapacity)
		n.eventChCancel = make(chan bool)
		go n.eventHandlerLoop()
	}

	uc, err := initState(n)
	if err != nil {
		return nil, err
	}
	// start a new round. if the node is behind then recovery will be started when a new UC arrives.
	n.startNewRound(uc)
	go n.loop()
	return n, nil
}

func initState(n *Node) (*certificates.UnicityCertificate, error) {
	// get genesis block from the genesis
	genesisBlock := n.configuration.genesisBlock()
	// latest block from the store
	latestPersistedBlock := n.blockStore.LatestBlock()
	var uc *certificates.UnicityCertificate
	if latestPersistedBlock != nil && latestPersistedBlock.BlockNumber > genesisBlock.BlockNumber {
		// restore from store
		prevBlock := genesisBlock
		for i := genesisBlock.BlockNumber + 1; i <= latestPersistedBlock.BlockNumber; i++ {
			bl, err := n.blockStore.Get(i)
			if err != nil {
				return nil, err
			}
			if !bytes.Equal(prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.PreviousBlockHash) {
				return nil, errors.Errorf("state init failed, invalid blockchain (previous block #%v hash='%X', current block #%v backlink='%X')", prevBlock.BlockNumber, prevBlock.UnicityCertificate.InputRecord.BlockHash, bl.BlockNumber, bl.PreviousBlockHash)
			}
			n.transactionSystem.BeginBlock(i)
			for _, tx := range bl.Transactions {
				gtx, err := n.transactionSystem.ConvertTx(tx)
				if err != nil {
					return nil, err
				}
				if err = n.validateAndExecuteTx(gtx); err != nil {
					return nil, err
				}
			}

			state, err := n.transactionSystem.EndBlock()
			if err != nil {
				return nil, err
			}
			uc = bl.UnicityCertificate
			if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
				return nil, errors.Errorf("invalid tx system state root of block #%v. expected: %X, got: %X", bl.BlockNumber, uc.InputRecord.Hash, state.Root())
			}
			n.transactionSystem.Commit()
			prevBlock = bl
		}
		logger.Info("State initialised from persistent store up to block #%v", prevBlock.BlockNumber)
	} else {
		if err := n.blockStore.Add(genesisBlock); err != nil {
			return nil, err
		}
		n.transactionSystem.Commit() // commit everything from the genesis
		uc = genesisBlock.UnicityCertificate
		logger.Info("State initialised from the genesis block")
	}
	return uc, nil
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
		case tx := <-n.txCh:
			n.process(tx)
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
					n.sendEvent(EventTypeError, err)
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
					n.sendEvent(EventTypeError, err)
					continue
				}
				n.sendEvent(EventTypeUnicityCertificateHandled, uc)
			case network.ProtocolBlockProposal:
				success, bp := convertType[*blockproposal.BlockProposal](m.Message)
				if !success {
					logger.Warning("Invalid block proposal type: %T", m.Message)
					continue
				}
				err := n.handleBlockProposal(bp)
				if err != nil {
					logger.Warning("Block proposal processing failed by node %v: %v", n.configuration.peer.ID(), err)
					n.sendEvent(EventTypeError, err)
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
		case <-n.timers.C:
			logger.Info("Handling T1 timeout")
			n.handleT1TimeoutEvent()
		}
	}
}

func (n *Node) sendEvent(eventType EventType, content any) {
	if n.eventHandler != nil {
		n.eventCh <- Event{
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
			n.eventHandler(e)
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

func (n *Node) startNewRound(uc *certificates.UnicityCertificate) {
	newBlockNr := n.blockStore.LatestBlock().BlockNumber + 1
	n.transactionSystem.BeginBlock(newBlockNr)
	n.proposal = nil
	n.pr = nil
	n.leaderSelector.UpdateLeader(uc.UnicitySeal)
	n.startHandleOrForwardTransactions()
	n.luc = uc
	n.timers.Restart(t1TimerName)
	n.sendEvent(EventTypeNewRoundStarted, newBlockNr)
}

func (n *Node) handleOrForwardTransaction(tx txsystem.GenericTransaction) bool {
	if err := n.txValidator.Validate(tx); err != nil {
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

func (n *Node) process(tx txsystem.GenericTransaction) {
	defer trackExecutionTime(time.Now(), "Processing transaction")
	if err := n.validateAndExecuteTx(tx); err != nil {
		return
	}
	n.proposal = append(n.proposal, tx)
	n.sendEvent(EventTypeTransactionProcessed, tx)
	logger.Debug("Transaction processed by node %v. Proposal size: %v", n.configuration.peer.ID(), len(n.proposal))
}

func (n *Node) validateAndExecuteTx(tx txsystem.GenericTransaction) error {
	if err := n.txValidator.Validate(tx); err != nil {
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
	defer trackExecutionTime(time.Now(), "Handling BlockProposal")
	if prop == nil {
		return blockproposal.ErrBlockProposalIsNil
	}
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
	blockNr := n.blockStore.LatestBlock().BlockNumber + 1
	n.transactionSystem.BeginBlock(blockNr)
	for _, tx := range prop.Transactions {
		genTx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			logger.Warning("transaction is invalid %v", err)
			continue
		}
		n.process(genTx)
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
	util.WriteDebugJsonLog(logger, "Received Unicity Certificate", uc)
	// UC is validated cryptographically
	if err := n.unicityCertificateValidator.Validate(uc); err != nil {
		logger.Warning("Invalid UnicityCertificate: %v", err)
		return errors.Errorf("invalid unicity certificate: %v", err)
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

	if n.pr == nil {
		// There is no pending block proposal. Start recovery unless the state is already up-to-date with UC.
		state, err := n.transactionSystem.EndBlock()
		if err != nil {
			return errors.Wrap(err, "tx system failed to end block")
		}
		if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			logger.Warning("UC's IR hash is different from state's root ('%X' vs '%X'", uc.InputRecord.Hash, state.Root())
			return n.startRecovery(uc)
		}
	} else if bytes.Equal(uc.InputRecord.Hash, n.pr.StateHash) {
		// UC certifies pending block proposal
		err := n.finalizeBlock(n.pr.Transactions, uc)
		if err != nil {
			return errors.Wrap(err, "block finalization failed")
		}
	} else if bytes.Equal(uc.InputRecord.Hash, n.pr.PrevHash) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		logger.Warning("Reverting state tree. UC IR hash: %X, proposal hash %X", uc.InputRecord.Hash, n.pr.PrevHash)
		n.transactionSystem.Revert()
		n.startNewRound(uc)
		return ErrStateReverted
	} else {
		// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
		logger.Warning("Reverting state tree.")
		n.transactionSystem.Revert()
		return n.startRecovery(uc)
	}
	n.startNewRound(uc)
	return nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(transactions []txsystem.GenericTransaction, uc *certificates.UnicityCertificate) error {
	defer trackExecutionTime(time.Now(), "Block finalization")
	latestBlock := n.blockStore.LatestBlock()
	newHeight := latestBlock.BlockNumber + 1
	logger.Info("Finalizing block #%v. TxCount: %v", newHeight, len(transactions))
	b := &block.Block{
		SystemIdentifier:   n.configuration.GetSystemIdentifier(),
		BlockNumber:        newHeight,
		PreviousBlockHash:  latestBlock.UnicityCertificate.InputRecord.BlockHash,
		Transactions:       toProtoBuf(transactions),
		UnicityCertificate: uc,
	}
	blockHash, err := b.Hash(n.configuration.hashAlgorithm)
	if err != nil {
		return err
	}
	if !bytes.Equal(blockHash, uc.InputRecord.BlockHash) {
		return errors.Errorf("finalized block hash not equal to IR block hash. IR hash %X, finalized block hash %X",
			uc.InputRecord.BlockHash, blockHash)
	}
	err = n.blockStore.Add(b)
	if err != nil {
		return err
	}
	n.transactionSystem.Commit()
	n.sendEvent(EventTypeBlockFinalized, b)
	return nil
}

func (n *Node) handleT1TimeoutEvent() {
	defer func() {
		n.leaderSelector.UpdateLeader(nil)
		n.stopForwardingOrHandlingTransactions()
	}()
	if n.leaderSelector.IsCurrentNodeLeader() {
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

	// TODO check recoveringNodeID is among known validators
	// n.configuration.peer.Validators()

	maxBlock, err := n.blockStore.Height()
	if err != nil {
		return errors.Wrap(err, "unable to fetch block height from the block store")
	}

	resp := &replication.LedgerReplicationResponse{}

	if !bytes.Equal(lr.GetSystemIdentifier(), n.configuration.GetSystemIdentifier()) {
		resp.Status = replication.LedgerReplicationResponse_UNKNOWN_SYSTEM_IDENTIFIER
		resp.Message = fmt.Sprintf("Unknown system identifier: %v", lr.GetSystemIdentifier())
	}

	if maxBlock < lr.GetBeginBlockNumber() {
		// nothing to recover, just return
		return nil
	} else if lr.GetEndBlockNumber() > lr.GetBeginBlockNumber() {
		maxBlock = lr.GetEndBlockNumber()
	}

	go func() {
		for i := lr.GetBeginBlockNumber(); i <= maxBlock; i++ {
			b, _ := n.blockStore.Get(i)
			resp.Blocks = []*block.Block{b} // TODO batch
			err := n.network.Send(network.OutputMessage{
				Protocol: network.ProtocolLedgerReplicationResp,
				Message:  resp,
			}, []peer.ID{recoveringNodeID})
			if err != nil {
				logger.Error("Problem sending ledger replication response, block #%d: %s", b.GetBlockNumber(), err)
			}
		}
	}()

	return nil
}

func (n *Node) handleLedgerReplicationResponse(lr *replication.LedgerReplicationResponse) error {
	util.WriteDebugJsonLog(logger, "Ledger replication response received", lr)
	logger.Debug("Recovery: latest node's block: #%v", n.GetLatestBlock().GetBlockNumber())

	if n.status == recovering {
		if lr.GetStatus() != replication.LedgerReplicationResponse_OK {
			return errors.Errorf("got erroneous Ledger Replication response, status=%v, message='%s'", lr.GetStatus(), lr.GetMessage())
		}
		blocks := lr.GetBlocks()
		for _, b := range blocks {
			logger.Debug("Recovering block #%v", b.GetBlockNumber())
			if !bytes.Equal(b.GetSystemIdentifier(), n.configuration.GetSystemIdentifier()) {
				return errors.Errorf("recovery failed: block %v contains invalid System ID: %x", b.GetBlockNumber(), b.GetSystemIdentifier())
			}
			//uc := b.GetUnicityCertificate()

			n.transactionSystem.BeginBlock(b.GetBlockNumber())
			for _, tx := range b.GetTransactions() {
				gtx, err := n.transactionSystem.ConvertTx(tx)
				if err != nil {
					return err
				}
				if err = n.validateAndExecuteTx(gtx); err != nil {
					return err
				}
			}

			// TODO
			_, err := n.transactionSystem.EndBlock()
			if err != nil {
				return errors.Wrapf(err, "recovery failed")
			}

			//if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			//	return errors.Errorf("recovery failed: IR's hash is not equal to state's hash (%X vs %X)", uc.InputRecord.Hash, state.Root())
			//} else if !bytes.Equal(uc.InputRecord.SummaryValue, state.Summary()) {
			//	return errors.Errorf("recovery failed: IR's summary value is not equal to tx system summary value (%X vs %X)", uc.InputRecord.SummaryValue, state.Summary())
			//}
			n.transactionSystem.Commit()
			if err = n.blockStore.Add(b); err != nil {
				return err
			}
		}
	} else {
		logger.Warning("Unexpected Ledger Replication response, node is not recovering", lr)
		// TODO ignore or fail?
	}

	// check if recovery is complete
	latestBlock := n.GetLatestBlock()
	logger.Debug("Checking if recovery is complete, latest block: #%v", latestBlock.GetBlockNumber())
	if latestBlock.GetUnicityCertificate().GetUnicitySeal().GetRootChainRoundNumber() == n.luc.GetUnicitySeal().GetRootChainRoundNumber() {
		//&&
		//	bytes.Equal(latestBlock.GetUnicityCertificate().GetUnicitySeal().GetHash(), n.luc.GetUnicitySeal().GetHash()) {
		logger.Info("Node is recovered until the given LUC, block '%X', root round: %v ", n.luc.GetInputRecord().GetBlockHash(), n.luc.GetUnicitySeal().GetRootChainRoundNumber())
		n.status = idle
		n.sendEvent(EventTypeRecoveryFinished, latestBlock.GetBlockNumber())
		//return n.handleUnicityCertificate(n.luc) TODO ?
	}
	return nil
}

func (n *Node) startRecovery(uc *certificates.UnicityCertificate) error {
	if n.status == recovering {
		// already recovering
		return nil
	}
	logger.Warning("Starting recovery")
	n.status = recovering
	n.stopForwardingOrHandlingTransactions()
	n.luc = uc // recover up to this UC
	fromBlockNr := n.blockStore.LatestBlock().GetBlockNumber() + 1
	n.sendEvent(EventTypeRecoveryStarted, fromBlockNr)
	go func() {
		err := n.sendLedgerReplicationRequest(fromBlockNr)
		if err != nil {
			logger.Warning("Error sending ledger replication request: %s", err)
		}
	}()
	return ErrNodeDoesNotHaveLatestBlock
}

func (n *Node) sendLedgerReplicationRequest(startingBlockNr uint64) error {
	req := &replication.LedgerReplicationRequest{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		NodeIdentifier:   n.leaderSelector.SelfID().String(),
		BeginBlockNumber: startingBlockNr,
	}

	util.WriteDebugJsonLog(logger, "Ledger replication request created", req)

	peers := n.configuration.peer.Validators()
	if len(peers) == 0 {
		return errors.Errorf("unable to send ledger replication request, no peers")
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

	return err
}

func (n *Node) sendBlockProposal() error {
	defer trackExecutionTime(time.Now(), "Sending BlockProposals")
	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.leaderSelector.SelfID()
	prop := &blockproposal.BlockProposal{
		SystemIdentifier:   systemIdentifier,
		NodeIdentifier:     nodeId.String(),
		UnicityCertificate: n.luc,
		Transactions:       toProtoBuf(n.proposal),
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
	prevHash := n.luc.InputRecord.Hash
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return errors.Wrap(err, "tx system failed to end block")
	}
	stateRoot := state.Root()
	summary := state.Summary()

	pendingProposal := &block.PendingBlockProposal{
		RoundNumber:  n.luc.UnicitySeal.RootChainRoundNumber,
		PrevHash:     prevHash,
		StateHash:    stateRoot,
		Transactions: n.proposal,
	}
	err = n.blockStore.AddPendingProposal(pendingProposal)
	if err != nil {
		return errors.Wrap(err, "failed to store pending block proposal")
	}
	n.pr = pendingProposal

	latestBlock := n.blockStore.LatestBlock()
	latestBlockHash := latestBlock.UnicityCertificate.InputRecord.BlockHash
	blockHash, err := n.hashProposedBlock(latestBlockHash, latestBlock.BlockNumber+1)
	if err != nil {
		return err
	}
	n.proposal = []txsystem.GenericTransaction{}

	req := &certification.BlockCertificationRequest{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId.String(),
		RootRoundNumber:  n.pr.RoundNumber,
		InputRecord: &certificates.InputRecord{
			PreviousHash: prevHash,
			Hash:         stateRoot,
			BlockHash:    blockHash,
			SummaryValue: summary,
		},
	}
	err = req.Sign(n.configuration.signer)
	if err != nil {
		return err
	}
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
	err = n.txValidator.Validate(genTx)
	if err != nil {
		return err
	}
	return n.txBuffer.Add(genTx)
}

func (n *Node) GetBlock(blockNr uint64) (*block.Block, error) {
	return n.blockStore.Get(blockNr)
}

func (n *Node) GetLatestBlock() *block.Block {
	return n.blockStore.LatestBlock()
}

func (n *Node) stopForwardingOrHandlingTransactions() {
	if n.txCtx != nil {
		n.txCancel()
		n.txWaitGroup.Wait()
		n.txCtx = nil
		n.txCancel = nil
	}
}

func (n *Node) startHandleOrForwardTransactions() {
	n.stopForwardingOrHandlingTransactions()
	leader := n.leaderSelector.GetLeaderID()
	if leader == UnknownLeader {
		return
	}
	n.txCtx, n.txCancel = context.WithCancel(context.Background())
	n.txWaitGroup.Add(1)
	go n.txBuffer.Process(n.txCtx, n.txWaitGroup, n.handleOrForwardTransaction)
}

func (n *Node) hashProposedBlock(prevBlockHash []byte, blockNumber uint64) ([]byte, error) {
	b := block.Block{
		SystemIdentifier:  n.configuration.GetSystemIdentifier(),
		BlockNumber:       blockNumber,
		PreviousBlockHash: prevBlockHash,
		Transactions:      toProtoBuf(n.pr.Transactions),
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
