package partition

import (
	"bytes"
	"context"
	"encoding/base64"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/timer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"
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
	ErrTxSystemIsNil                    = errors.New("transaction system is nil")
	ErrEventBusIsNil                    = errors.New("event bus is nil")
	ErrPartitionConfigurationIsNil      = errors.New("configuration is nil")
	ErrLeaderSelectorIsNil              = errors.New("leader selector is nil")
	ErrCtxIsNil                         = errors.New("ctx is nil")
	ErrUnicityCertificateValidatorIsNil = errors.New("unicity certificate validator is nil")
	ErrTxValidatorIsNil                 = errors.New("tx validator is nil")
	ErrBlockStoreIsNil                  = errors.New("block store is nil")
	ErrGenesisIsNil                     = errors.New("genesis is nil")
	ErrInvalidRootHash                  = errors.New("tx system root hash does not equal to genesis file hash")
	ErrInvalidSummaryValue              = errors.New("tx system summary value does not equal to genesis file summary value")
	ErrNodeDoesNotHaveLatestBlock       = errors.New("node does not have latest block")
	ErrStateReverted                    = errors.New("state reverted")
)

type (

	//TODO idea: maybe we can use a block struct instead (needs a spec change)? (AB-119)
	pendingBlockProposal struct {
		lucRoundNumber uint64
		lucIRHash      []byte
		stateHash      []byte
		Transactions   []*transaction.Transaction
	}

	// Partition is a distributed system, it consists of either a set of shards, or one or more partition nodes.
	// Partition implements an instance of a specific TransactionSystem.
	Partition struct {
		transactionSystem           txsystem.TransactionSystem
		configuration               *Configuration
		luc                         *certificates.UnicityCertificate
		proposal                    []*transaction.Transaction
		pr                          *pendingBlockProposal
		timers                      *timer.Timers
		leaderSelector              LeaderSelector
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockStore                  store.BlockStore
		status                      status
		unicityCertificatesCh       <-chan interface{}
		transactionsCh              <-chan interface{}
		blockProposalsCh            <-chan interface{}
		eventbus                    *eventbus.EventBus
		ctx                         context.Context
		ctxCancel                   context.CancelFunc
	}

	status int
)

// New creates a new instance of Partition component.
func New(
	ctx context.Context,
	txSystem txsystem.TransactionSystem,
	eb *eventbus.EventBus,
	leaderSelector LeaderSelector,
	ucValidator UnicityCertificateValidator,
	txValidator TxValidator,
	blockStore store.BlockStore,
	configuration *Configuration,
) (*Partition, error) {
	if ctx == nil {
		return nil, ErrCtxIsNil
	}
	if txSystem == nil {
		return nil, ErrTxSystemIsNil
	}
	if configuration == nil {
		return nil, ErrPartitionConfigurationIsNil
	}
	partitionGenesis := configuration.Genesis
	if partitionGenesis == nil {
		return nil, ErrGenesisIsNil
	}
	if eb == nil {
		return nil, ErrEventBusIsNil
	}
	if ucValidator == nil {
		return nil, ErrUnicityCertificateValidatorIsNil
	}
	if txValidator == nil {
		return nil, ErrTxValidatorIsNil
	}
	if blockStore == nil {
		return nil, ErrBlockStoreIsNil
	}
	if leaderSelector == nil {
		return nil, ErrLeaderSelectorIsNil
	}

	unicityCertificatesCh, err := eb.Subscribe(eventbus.TopicPartitionUnicityCertificate, defaultTopicCapacity)
	if err != nil {
		return nil, err
	}
	transactionsCh, err := eb.Subscribe(eventbus.TopicPartitionTransaction, defaultTopicCapacity)
	if err != nil {
		return nil, err
	}
	blockProposalsCh, err := eb.Subscribe(eventbus.TopicBlockProposalInput, defaultTopicCapacity)
	if err != nil {
		return nil, err
	}

	if err := partitionGenesis.IsValid(configuration.TrustBase, configuration.HashAlgorithm); err != nil {
		logger.Warning("Invalid partition genesis file: %v", err)
		return nil, errors.Wrap(err, "invalid root partition genesis file")
	}

	state := txSystem.Init()
	txGenesisRoot := state.Root()
	txSummaryValue := state.Summary()
	genesisCertificate := partitionGenesis.Certificate
	genesisInputRecord := genesisCertificate.InputRecord
	if !bytes.Equal(genesisInputRecord.Hash, txGenesisRoot) {
		logger.Warning("tx system root hash does not equal to genesis file hash. "+
			"genesis hash: %X, txSystem hash: %X", genesisInputRecord.Hash, txGenesisRoot)
		return nil, ErrInvalidRootHash
	}

	if !bytes.Equal(genesisInputRecord.SummaryValue, txSummaryValue) {
		logger.Warning("tx system summary value does not equal to genesis file summary value. "+
			"Genesis SummaryValue: %X, TxSystem SummaryValue: %X", genesisInputRecord.SummaryValue, txSummaryValue)
		return nil, ErrInvalidSummaryValue
	}

	timers := timer.NewTimers()
	timers.Start(t1TimerName, configuration.T1Timeout)

	p := &Partition{
		transactionSystem:           txSystem,
		configuration:               configuration,
		timers:                      timers,
		leaderSelector:              leaderSelector,
		eventbus:                    eb,
		unicityCertificatesCh:       unicityCertificatesCh,
		transactionsCh:              transactionsCh,
		blockProposalsCh:            blockProposalsCh,
		unicityCertificateValidator: ucValidator,
		txValidator:                 txValidator,
		blockStore:                  blockStore,
	}
	p.ctx, p.ctxCancel = context.WithCancel(ctx)

	genesisBlock := &block.Block{
		SystemIdentifier:    p.configuration.GetSystemIdentifier(),
		TxSystemBlockNumber: 1,
		PreviousBlockHash:   nil,
		Transactions:        []*transaction.Transaction{},
		UnicityCertificate:  genesisCertificate,
	}
	if err := p.blockStore.Add(genesisBlock); err != nil {
		return nil, err
	}
	p.status = idle
	// start a new round. if the node is behind then recovery will be started when a new UC arrives.
	p.startNewRound(genesisCertificate)
	go p.loop()

	return p, nil
}

// Close shuts down the Partition component.
func (p *Partition) Close() {
	p.status = closing
	p.timers.WaitClose()
	p.ctxCancel()
}

// loop handles messages from different goroutines.
func (p *Partition) loop() {
	for {
		select {
		case <-p.ctx.Done():
			logger.Info("Exiting partition component main loop")
			return
		case tx := <-p.transactionsCh:
			logger.Info("Handling tx event %v", tx)
			p.handleTxEvent(tx)
		case e := <-p.unicityCertificatesCh:
			logger.Info("Handling unicity certificate %v", e)
			p.handleUnicityCertificateEvent(e)
		case e := <-p.blockProposalsCh:
			logger.Info("Handling block proposal %v", e)
			success, proposal := convertType[eventbus.BlockProposalEvent](e)
			if !success {
				logger.Warning("Invalid Block proposal event type: %v", e)
				return
			}
			err := p.handleBlockProposal(proposal)
			if err != nil {
				logger.Warning("Block proposal processing failed: %v", err)
			}
		case <-p.timers.C:
			logger.Info("Handling T1 timeout event")
			p.handleT1TimeoutEvent()
		}
	}
}
func (p *Partition) GetCurrentProposal() []*transaction.Transaction {
	return p.proposal
}

func (p *Partition) startNewRound(uc *certificates.UnicityCertificate) {
	p.transactionSystem.BeginBlock()
	p.proposal = nil
	p.pr = nil
	p.leaderSelector.UpdateLeader(uc.UnicitySeal)
	p.luc = uc

	p.timers.Restart(t1TimerName)
}

func (p *Partition) handleTxEvent(event interface{}) {
	switch event.(type) {
	case eventbus.TransactionEvent:
		p.process(event.(eventbus.TransactionEvent).Transaction)
	default:
		logger.Warning("Invalid event: %v", event)
	}
}

func (p *Partition) process(tx *transaction.Transaction) {
	if p.status == closing {
		return
	}
	if err := p.txValidator.Validate(tx); err != nil {
		logger.Warning("Transaction '%v' is invalid: %v", tx, err)
		return
	}
	if err := p.transactionSystem.Execute(tx); err != nil {
		logger.Warning("Tx was '%v' ignored by txSystem: %v", tx, err)
		return
	}
	p.proposal = append(p.proposal, tx)
	logger.Debug("Transaction processed. ProposalSize: %v", len(p.proposal))
}

func (p *Partition) handleUnicityCertificateEvent(event interface{}) {
	switch event.(type) {
	case eventbus.UnicityCertificateEvent:
		err := p.handleUnicityCertificate(event.(eventbus.UnicityCertificateEvent).Certificate)
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
func (p *Partition) handleBlockProposal(event eventbus.BlockProposalEvent) error {

	return nil
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
func (p *Partition) handleUnicityCertificate(uc *certificates.UnicityCertificate) error {
	// UC is validated cryptographically
	if err := p.unicityCertificateValidator.Validate(uc); err != nil {
		logger.Warning("Invalid UnicityCertificate: %v", err)
		return errors.Errorf("invalid unicity certificate: %v", err)
	}
	// UC must be newer than the last one seen
	if uc.UnicitySeal.RootChainRoundNumber < p.luc.UnicitySeal.RootChainRoundNumber {
		logger.Warning("Received UC is older than LUC. UC round Number:  %v, LUC round number: %v",
			uc.UnicitySeal.RootChainRoundNumber, p.luc.UnicitySeal.RootChainRoundNumber)
		return errors.Errorf("received UC is older than LUC. uc round %v, luc round %v",
			uc.UnicitySeal.RootChainRoundNumber, p.luc.UnicitySeal.RootChainRoundNumber)
	}

	// there can not be two UC-s with the same Root Chain block number but certifying different state root hashes.
	if uc.UnicitySeal.RootChainRoundNumber == p.luc.UnicitySeal.RootChainRoundNumber &&
		!bytes.Equal(uc.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Got two UC-s with the same Root Chain block number but certifying different state root "+
			"hashes. RootChainNumber: %v, UC IR hash: %X, LUC IR hash: %X",
			uc.UnicitySeal.RootChainRoundNumber,
			uc.InputRecord.Hash,
			p.luc.InputRecord.Hash,
		)
		return errors.Errorf("equivocating certificates: round number %v, received IR hash %X, latest IR hash %X",
			uc.UnicitySeal.RootChainRoundNumber, uc.InputRecord.Hash, p.luc.InputRecord.Hash)
	}

	// there can not be two UC-s extending the same state, but certifying different states (forking).
	if bytes.Equal(uc.InputRecord.PreviousHash, p.luc.InputRecord.PreviousHash) &&
		!bytes.Equal(p.luc.InputRecord.PreviousHash, p.luc.InputRecord.Hash) && // exclude empty blocks
		!bytes.Equal(uc.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Got two UC-s extending the same state, but certifying different states. "+
			"PreviousHash: %X, UC IR hash: %X, LUC IR hash: %X",
			uc.InputRecord.PreviousHash,
			uc.InputRecord.Hash,
			p.luc.InputRecord.Hash,
		)
		return errors.Errorf("equivocating certificates. previous IR hash %X, received IR hash %X, "+
			"latest IR hash %X", uc.InputRecord.PreviousHash, uc.InputRecord.Hash, p.luc.InputRecord.Hash)
	}

	if p.pr == nil {
		// There is no pending block proposal. Start recovery unless the state is already up-to-date with UC.
		state := p.transactionSystem.EndBlock()
		if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			logger.Warning("Starting recovery")
			p.status = recovering
			// TODO start recovery (AB-41)
			return ErrNodeDoesNotHaveLatestBlock
		}
	} else if bytes.Equal(uc.InputRecord.Hash, p.pr.stateHash) {
		// UC certifies pending block proposal
		p.finalizeBlock(p.pr.Transactions, uc)
	} else if bytes.Equal(uc.InputRecord.Hash, p.pr.lucIRHash) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		logger.Warning("Reverting state tree. UC IR hash: %X, proposal hash", uc.InputRecord.Hash, p.pr.lucIRHash)
		p.transactionSystem.Revert()
		p.startNewRound(uc)
		return ErrStateReverted
	} else {
		// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
		logger.Warning("Reverting state tree.")
		p.transactionSystem.Revert()
		logger.Warning("Starting recovery.")
		p.status = recovering
		// TODO start recovery (AB-41)
		return ErrNodeDoesNotHaveLatestBlock
	}
	p.startNewRound(uc)
	return nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (p *Partition) finalizeBlock(transactions []*transaction.Transaction, uc *certificates.UnicityCertificate) {
	logger.Info("Finalizing block. TxCount: %v", len(transactions))
	height, _ := p.blockStore.Height()           // TODO handle error
	latestBlock, _ := p.blockStore.LatestBlock() // TODO handle error
	b := &block.Block{
		SystemIdentifier:    p.configuration.GetSystemIdentifier(),
		TxSystemBlockNumber: height + 1,
		PreviousBlockHash:   latestBlock.Hash(p.configuration.HashAlgorithm),
		Transactions:        transactions,
		UnicityCertificate:  uc,
	}
	// TODO ensure block hash equals to IR hash
	_ = p.blockStore.Add(b) // TODO handle error
	p.transactionSystem.Commit()
}

func (p *Partition) handleT1TimeoutEvent() {
	if p.leaderSelector.IsCurrentNodeLeader() {
		p.sendProposal()
	}
	p.leaderSelector.UpdateLeader(nil)
}

func (p *Partition) sendProposal() {
	logger.Info("Sending BlockProposal. TxCount: %v", len(p.proposal))
	systemIdentifier := p.configuration.GetSystemIdentifier()
	nodeId := p.leaderSelector.SelfID()
	err := p.eventbus.Submit(eventbus.TopicBlockProposalOutput, &eventbus.BlockProposalEvent{
		SystemIdentifier:   systemIdentifier,
		NodeIdentifier:     nodeId,
		UnicityCertificate: p.luc,
		Transactions:       p.proposal,
	})
	if err != nil {
		logger.Warning("Failed to send BlockProposal event: %v", err)
		return
	}
	prevHash := p.luc.InputRecord.Hash
	logger.Debug("Previous IR hash %v", base64.StdEncoding.EncodeToString(prevHash))
	state := p.transactionSystem.EndBlock()
	stateRoot := state.Root()
	summary := state.Summary()
	p.pr = &pendingBlockProposal{
		lucRoundNumber: p.luc.UnicitySeal.RootChainRoundNumber,
		lucIRHash:      prevHash,
		stateHash:      stateRoot,
		Transactions:   p.proposal,
	}

	// TODO store pending block proposal (AB-132)

	height, _ := p.blockStore.Height() // TODO handle error
	blockNr := height + 1

	latestBlock, _ := p.blockStore.LatestBlock() // TODO handle error
	prevBlockHash := latestBlock.Hash(p.configuration.HashAlgorithm)

	hasher := p.configuration.HashAlgorithm.New()
	hasher.Write(p.configuration.GetSystemIdentifier())
	hasher.Write(util.Uint64ToBytes(blockNr))
	hasher.Write(prevBlockHash)

	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	blockHash := hasher.Sum(nil)

	p.proposal = []*transaction.Transaction{}

	event := eventbus.P1Event{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId,
		LucRoundNumber:   p.pr.lucRoundNumber,
		InputRecord: &certificates.InputRecord{
			PreviousHash: prevHash,
			Hash:         stateRoot,
			BlockHash:    blockHash,
			SummaryValue: summary,
		},
	}
	logger.Info("Sending P1 event")
	err = p.eventbus.Submit(eventbus.TopicP1, event)
	if err != nil {
		logger.Warning("Failed to send P1 event: %v", err)
	}
}

func convertType[T any](event interface{}) (bool, T) {
	var result T
	switch event.(type) {
	case T:
		return true, event.(T)
	}
	return false, result
}
