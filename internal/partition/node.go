package partition

import (
	"bytes"
	"context"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/timer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txbuffer"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"
)

const (
	idle status = iota
	recovering
	closing
)

const (
	t1TimerName          = "t1"
	defaultTopicCapacity = 100
)

var (
	ErrNodeDoesNotHaveLatestBlock = errors.New("node does not have the latest block")
	ErrStateReverted              = errors.New("state reverted")
)

type (
	pendingProposal struct {
		roundNumber  uint64
		prevHash     []byte
		stateHash    []byte
		Transactions []txsystem.GenericTransaction
	}

	// Node represents a member in the partition and implements an instance of a specific TransactionSystem. Partition
	// is a distributed system, it consists of either a set of shards, or one or more partition nodes.
	Node struct {
		status                      status
		configuration               *configuration
		transactionSystem           txsystem.TransactionSystem
		luc                         *certificates.UnicityCertificate
		proposal                    []txsystem.GenericTransaction
		pr                          *pendingProposal
		timers                      *timer.Timers
		leaderSelector              LeaderSelector
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  store.BlockStore
		txBuffer                    *txbuffer.TxBuffer

		unicityCertificatesCh <-chan interface{}
		transactionsCh        <-chan interface{}
		blockProposalsCh      <-chan interface{}
		eventbus              *eventbus.EventBus
		ctx                   context.Context
		ctxCancel             context.CancelFunc
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
	nodeOptions ...NodeOption, // additional optional configuration parameters
) (*Node, error) {
	// load and validate node configuration
	conf, err := loadAndValidateConfiguration(peer, signer, genesis, txSystem, nodeOptions...)
	if err != nil {
		return nil, err
	}

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
		eventbus:                    conf.eventbus,
	}
	n.ctx, n.ctxCancel = context.WithCancel(conf.context)

	// subscribe topics
	n.unicityCertificatesCh, n.transactionsCh, n.blockProposalsCh, err = subscribeTopics(conf.eventbus)
	if err != nil {
		return nil, err
	}

	// init timer
	n.timers = timer.NewTimers()
	n.timers.Start(t1TimerName, conf.t1Timeout)

	// get genesis block from the genesis
	genesisBlock := conf.genesisBlock()
	if err := n.blockStore.Add(genesisBlock); err != nil {
		return nil, err
	}
	txSystem.Commit() // commit everything from the genesis
	// start a new round. if the node is behind then recovery will be started when a new UC arrives.
	n.startNewRound(genesisBlock.UnicityCertificate)
	go n.loop()
	return n, nil
}

// Close shuts down the Node component.
func (n *Node) Close() {
	n.status = closing
	for _, processor := range n.configuration.processors {
		processor.Close()
	}
	n.timers.WaitClose()
	n.ctxCancel()
}

// loop handles messages from different goroutines.
func (n *Node) loop() {
	for {
		select {
		case <-n.ctx.Done():
			logger.Info("Exiting partition node component main loop")
			return
		case tx := <-n.transactionsCh:
			if n.status == closing {
				continue
			}
			util.WriteDebugJsonLog(logger, "Handling tx event", tx)
			n.handleTxEvent(tx)
		case e := <-n.unicityCertificatesCh:
			if n.status == closing {
				continue
			}

			util.WriteDebugJsonLog(logger, "Handling unicity certificate", e)
			n.handleUnicityCertificateEvent(e)
		case e := <-n.blockProposalsCh:
			if n.status == closing {
				continue
			}
			util.WriteDebugJsonLog(logger, "Handling block proposal", e)
			success, proposalEvent := convertType[eventbus.BlockProposalEvent](e)
			if !success {
				logger.Warning("Invalid Block proposal event type: %v", e)
				continue
			}
			err := n.handleBlockProposal(proposalEvent.BlockProposal)
			if err != nil {
				logger.Warning("Block proposal processing failed: %v", err)
			}
		case <-n.timers.C:
			if n.status == closing {
				continue
			}
			logger.Info("Handling T1 timeout")
			n.handleT1TimeoutEvent()
		}
	}
}

func (n *Node) startNewRound(uc *certificates.UnicityCertificate) {
	n.transactionSystem.BeginBlock(n.blockStore.LatestBlock().BlockNumber + 1)
	n.proposal = nil
	n.pr = nil
	n.leaderSelector.UpdateLeader(uc.UnicitySeal)
	n.luc = uc
	n.timers.Restart(t1TimerName)
}

func (n *Node) handleTxEvent(event interface{}) {
	switch event.(type) {
	case eventbus.TransactionEvent:

		tx := event.(eventbus.TransactionEvent).Transaction
		genTx, err := n.transactionSystem.ConvertTx(tx)
		if err != nil {
			logger.Warning("Failed to convert tx: %v", err)
			return
		}
		n.process(genTx)
	default:
		logger.Warning("Invalid event: %v", event)
	}
}

func (n *Node) process(tx txsystem.GenericTransaction) {
	defer trackExecutionTime(time.Now(), "Processing transaction")
	if err := n.txValidator.Validate(tx); err != nil {
		logger.Warning("Transaction '%v' is invalid: %v", tx, err)
		return
	}
	if err := n.transactionSystem.Execute(tx); err != nil {
		logger.Warning("TxSystem was unable to process transaction '%v': %v", tx, err)
		return
	}
	n.proposal = append(n.proposal, tx)
	logger.Debug("Transaction processed. Proposal size: %v", len(n.proposal))
}

func (n *Node) handleUnicityCertificateEvent(event interface{}) {
	switch event.(type) {
	case eventbus.UnicityCertificateEvent:
		err := n.handleUnicityCertificate(event.(eventbus.UnicityCertificateEvent).Certificate)
		if err != nil {
			logger.Warning("Unicity Certificate processing failed: %v", err)
		}
	default:
		logger.Warning("Invalid unicity certificate event: %v", event)
	}
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
//  7. Certificate Request query (protocol P1 query) is assembled and sent to the Root Chain.
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
	n.transactionSystem.BeginBlock(n.blockStore.LatestBlock().BlockNumber + 1)
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
			logger.Warning("Starting recovery")
			n.status = recovering
			// TODO start recovery (AB-41)
			return ErrNodeDoesNotHaveLatestBlock
		}
	} else if bytes.Equal(uc.InputRecord.Hash, n.pr.stateHash) {
		// UC certifies pending block proposal
		n.finalizeBlock(n.pr.Transactions, uc)
	} else if bytes.Equal(uc.InputRecord.Hash, n.pr.prevHash) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		logger.Warning("Reverting state tree. UC IR hash: %X, proposal hash %X", uc.InputRecord.Hash, n.pr.prevHash)
		n.transactionSystem.Revert()
		n.startNewRound(uc)
		return ErrStateReverted
	} else {
		// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
		logger.Warning("Reverting state tree.")
		n.transactionSystem.Revert()
		logger.Warning("Starting recovery.")
		n.status = recovering
		// TODO start recovery (AB-41)
		return ErrNodeDoesNotHaveLatestBlock
	}
	n.startNewRound(uc)
	return nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(transactions []txsystem.GenericTransaction, uc *certificates.UnicityCertificate) {
	defer trackExecutionTime(time.Now(), "Block finalization")
	logger.Info("Finalizing block. TxCount: %v", len(transactions))
	height := n.blockStore.LatestBlock().BlockNumber
	latestBlock := n.blockStore.LatestBlock()
	b := &block.Block{
		SystemIdentifier:   n.configuration.GetSystemIdentifier(),
		BlockNumber:        height + 1,
		PreviousBlockHash:  latestBlock.Hash(n.configuration.hashAlgorithm),
		Transactions:       toProtoBuf(transactions),
		UnicityCertificate: uc,
	}
	// TODO ensure block hash equals to IR hash
	_ = n.blockStore.Add(b) // TODO handle error
	n.transactionSystem.Commit()
}

func (n *Node) handleT1TimeoutEvent() {
	defer n.leaderSelector.UpdateLeader(nil)
	if n.leaderSelector.IsCurrentNodeLeader() {
		if err := n.sendBlockProposal(); err != nil {
			logger.Warning("Failed to send BlockProposal event: %v", err)
			return
		}
		if err := n.sendCertificationRequest(); err != nil {
			logger.Warning("Failed to send certification request: %v", err)
		}
	} else {
		logger.Debug("Current node is not the leader.")
	}
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
	util.WriteDebugJsonLog(logger, "New BlockProposal created", prop)
	err := prop.Sign(n.configuration.hashAlgorithm, n.configuration.signer)
	if err != nil {
		return err
	}
	return n.eventbus.Submit(eventbus.TopicBlockProposalOutput, eventbus.BlockProposalEvent{BlockProposal: prop})
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

	height, err := n.blockStore.Height()
	if err != nil {
		return err
	}
	blockNr := height + 1

	latestBlock := n.blockStore.LatestBlock()
	prevBlockHash := latestBlock.Hash(n.configuration.hashAlgorithm)

	n.pr = &pendingProposal{
		roundNumber:  n.luc.UnicitySeal.RootChainRoundNumber,
		prevHash:     prevHash,
		stateHash:    stateRoot,
		Transactions: n.proposal,
	}
	// TODO store pending block proposal (AB-132)

	hasher := n.configuration.hashAlgorithm.New()
	hasher.Write(n.configuration.GetSystemIdentifier())
	hasher.Write(util.Uint64ToBytes(blockNr))
	hasher.Write(prevBlockHash)

	for _, tx := range n.pr.Transactions {
		hasher.Write(tx.Hash(n.configuration.hashAlgorithm))
	}
	blockHash := hasher.Sum(nil)

	n.proposal = []txsystem.GenericTransaction{}

	req := &p1.P1Request{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId.String(),
		RootRoundNumber:  n.pr.roundNumber,
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
	event := eventbus.BlockCertificationEvent{
		Req: req,
	}
	return n.eventbus.Submit(eventbus.TopicP1, event)
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

func subscribeTopics(eb *eventbus.EventBus) (<-chan interface{}, <-chan interface{}, <-chan interface{}, error) {
	unicityCertificatesCh, err := eb.Subscribe(eventbus.TopicPartitionUnicityCertificate, defaultTopicCapacity)
	if err != nil {
		return nil, nil, nil, err
	}
	transactionsCh, err := eb.Subscribe(eventbus.TopicPartitionTransaction, defaultTopicCapacity)
	if err != nil {
		return nil, nil, nil, err
	}
	blockProposalsCh, err := eb.Subscribe(eventbus.TopicBlockProposalInput, defaultTopicCapacity)
	if err != nil {
		return nil, nil, nil, err
	}
	return unicityCertificatesCh, transactionsCh, blockProposalsCh, nil
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
