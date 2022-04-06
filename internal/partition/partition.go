package partition

import (
	"bytes"
	"context"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

const (
	idle status = iota
	recovering
	closing
)
const topicCapacity = 100

var (
	ErrTxSystemIsNil               = errors.New("transaction system is nil")
	ErrEventBusIsNil               = errors.New("event bus is nil")
	ErrPartitionConfigurationIsNil = errors.New("configuration is nil")
	ErrLeaderSelectorIsNil         = errors.New("leader selector is nil")
	ErrCtxIsNil                    = errors.New("ctx is nil")
	ErrUcrValidatorIsNil           = errors.New("unicity certificate record validator is nil")
	ErrTxValidatorIsNil            = errors.New("tx validator is nil")
	ErrBlockStoreIsNil             = errors.New("block store is nil")
	ErrGenesisIsNil                = errors.New("genesis is nil")
)

type (
	UnicityCertificate struct {
		InputRecord            *certificates.InputRecord
		UnicityTreeCertificate *certificates.UnicityTreeCertificate
		UnicityCertificate     *UnicitySeal
	}

	UnicitySeal struct {
		RootChainBlockNumber uint64
		PreviousHash         []byte // Previous Block hash
		Hash                 []byte // Block hash
		// TODO signatures (AB-131)
	}

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
		transactionSystem                 TransactionSystem
		configuration                     *Configuration
		luc                               *UnicityCertificate
		proposal                          []*transaction.Transaction
		pr                                *pendingBlockProposal
		t1                                *time.Timer
		leaderSelector                    *LeaderSelector
		txValidator                       TxValidator
		unicityCertificateRecordValidator UnicityCertificateRecordValidator
		blockStore                        BlockStore
		status                            status
		unicityCertificatesCh             <-chan interface{}
		transactionsCh                    <-chan interface{}
		eventbus                          *eventbus.EventBus
		ctx                               context.Context
		ctxCancel                         context.CancelFunc
	}

	status int
)

// New creates a new instance of Partition component.
func New(
	ctx context.Context,
	txSystem TransactionSystem,
	eb *eventbus.EventBus,
	leaderSelector *LeaderSelector,
	ucrValidator UnicityCertificateRecordValidator,
	txValidator TxValidator,
	blockStore BlockStore,
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
	if configuration.Genesis == nil {
		return nil, ErrGenesisIsNil
	}
	if eb == nil {
		return nil, ErrEventBusIsNil
	}
	if ucrValidator == nil {
		return nil, ErrUcrValidatorIsNil
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

	t1 := time.NewTimer(configuration.T1Timeout)
	stopTimer(t1)

	unicityCertificatesCh, err := eb.Subscribe(TopicPartitionUnicityCertificate, topicCapacity)
	if err != nil {
		return nil, err
	}
	transactionsCh, err := eb.Subscribe(TopicPartitionTransaction, topicCapacity)
	if err != nil {
		return nil, err
	}

	context, cancelFunc := context.WithCancel(ctx)

	p := &Partition{
		ctx:                               context,
		ctxCancel:                         cancelFunc,
		transactionSystem:                 txSystem,
		configuration:                     configuration,
		t1:                                t1,
		leaderSelector:                    leaderSelector,
		eventbus:                          eb,
		unicityCertificatesCh:             unicityCertificatesCh,
		transactionsCh:                    transactionsCh,
		unicityCertificateRecordValidator: ucrValidator,
		txValidator:                       txValidator,
		blockStore:                        blockStore,
	}

	txGenesisRoot, genesisSummaryValue := p.transactionSystem.RCompl()
	if !bytes.Equal(configuration.Genesis.InputRecord.Hash, txGenesisRoot) {
		logger.Warning("tx system root hash does not equal to genesis file hash")
		//TODO AB-111
		//return nil, errors.Errorf("tx system root hash does not equal to genesis file hash")
	}

	if !bytes.Equal(configuration.Genesis.InputRecord.SummaryValue, genesisSummaryValue.Bytes()) {
		logger.Warning("tx system summary value does not equal to genesis file summary value")
		//TODO AB-111
		//return nil, errors.Errorf("tx system summary value does not equal to genesis file summary value")
	}

	if err = ucrValidator.Validate(configuration.Genesis.UnicityCertificateRecord); err != nil {
		logger.Warning("invalid genesis unicity certificate record. %v", err)
		return nil, err
	}
	genesisBlock := &Block{
		SystemIdentifier:    p.configuration.SystemIdentifier,
		TxSystemBlockNumber: 1,
		PreviousBlockHash:   nil,
		Transactions:        []*transaction.Transaction{},
		UnicityCertificate:  configuration.Genesis.UnicityCertificateRecord,
	}
	err = p.blockStore.Add(genesisBlock)
	if err != nil {
		return nil, err
	}
	p.status = idle
	// start a new round. if the node is behind then recovery will be started when a new UC arrives.
	err = p.startNewRound(configuration.Genesis.UnicityCertificateRecord)
	if err != nil {
		return nil, err
	}
	go p.loop()

	return p, nil
}

// Close shuts down the Partition component.
func (p *Partition) Close() {
	p.status = closing
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
			p.handleUnicityCertificateRecordEvent(e)
		case <-p.t1.C:
			logger.Info("Handling T1 timeout event")
			p.handleT1TimeoutEvent()
		}
		// TODO handle block proposals (AB-119)
	}
}

func (p *Partition) startNewRound(ucr *UnicityCertificate) error {
	p.transactionSystem.RInit()
	p.proposal = nil
	p.pr = nil
	err := p.leaderSelector.UpdateLeader(ucr.UnicityCertificate)
	if err != nil {
		return err
	}
	p.luc = ucr
	p.resetTimer()
	return nil
}

func (p *Partition) handleTxEvent(event interface{}) {
	switch event.(type) {
	case TransactionEvent:
		p.process(event.(TransactionEvent).Transaction)
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
	logger.Debug("Transaction '%v' successfully processed", tx)
}

func (p *Partition) handleUnicityCertificateRecordEvent(event interface{}) {
	switch event.(type) {
	case UnicityCertificateRecordEvent:
		p.handleUnicityCertificateRecord(event.(UnicityCertificateRecordEvent).Certificate)
	default:
		logger.Warning("Invalid event: %v", event)
	}
}

func (p *Partition) handleUnicityCertificateRecord(ucr *UnicityCertificate) {
	// TODO refactor and write tests after task AB-130 is done
	if ucr == nil {
		logger.Warning("Invalid UnicityCertificateRecordEvent. UC is nil")
		return
	}

	if err := p.unicityCertificateRecordValidator.Validate(ucr); err != nil {
		logger.Warning("Invalid UnicityCertificate. Invalid system identifier. Expected: %X, got %X", p.configuration.SystemIdentifier, ucr.UnicityTreeCertificate.SystemIdentifier)
		return
	}

	// ensure(uc.Cuni.α = α)
	if !bytes.Equal(ucr.UnicityTreeCertificate.SystemIdentifier, p.configuration.SystemIdentifier) {
		logger.Warning("Invalid UnicityCertificate. Invalid system identifier. Expected: %X, got %X", p.configuration.SystemIdentifier, ucr.UnicityTreeCertificate.SystemIdentifier)
		return
	}
	// ensure(¬(uc.UC.nr = luc.UC.nr ∧ uc.IR.h , luc.IR.h))
	if ucr.UnicityCertificate.RootChainBlockNumber == p.luc.UnicityCertificate.RootChainBlockNumber &&
		!bytes.Equal(ucr.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Equivocating certificates. UC: %v,LUC: %v", ucr, p.luc)
		return
	}
	// ensure(¬(uc.IR.h′ = luc.IR.h′ ∧ uc.IR.h , luc.IR.h))
	if bytes.Equal(ucr.InputRecord.PreviousHash, p.luc.InputRecord.PreviousHash) &&
		!bytes.Equal(ucr.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Equivocating certificates. UC: %v,LUC: %v", ucr, p.luc)
		return
	}
	//ensure(uc.UC.nr > luc.UC.nr)
	if ucr.UnicityCertificate.RootChainBlockNumber < p.luc.UnicityCertificate.RootChainBlockNumber {
		logger.Warning("Received UC is older than LUC. UC:  %v,LUC: %v", ucr, p.luc)
		return
	}
	if p.pr == nil {
		hash, _ := p.transactionSystem.RCompl()
		if !bytes.Equal(ucr.InputRecord.Hash, hash) {
			logger.Warning("Starting recovery. UC: %v,LUC: %v", ucr, p.luc)
			p.status = recovering
			// TODO start recovery (AB-41)
			return
		}
	} else if bytes.Equal(ucr.InputRecord.Hash, p.pr.stateHash) {
		logger.Info("Finalizing block")
		p.finalizeBlock(p.pr.Transactions, ucr)
	} else if bytes.Equal(ucr.InputRecord.Hash, p.pr.lucIRHash) {
		logger.Warning("Reverting state tree. UC: %v,LUC: %v", ucr, p.luc)
		p.transactionSystem.Revert()
	} else {
		logger.Warning("Reverting state tree. UC: %v,LUC: %v", ucr, p.luc)
		p.transactionSystem.Revert()
		logger.Warning("Starting recovery. UC: %v,LUC: %v", ucr, p.luc)
		p.status = recovering
		// TODO start recovery (AB-41)
	}
	p.startNewRound(ucr)
}

func (p *Partition) finalizeBlock(transactions []*transaction.Transaction, ucr *UnicityCertificate) {
	height, _ := p.blockStore.Height()           // TODO handle error
	latestBlock, _ := p.blockStore.LatestBlock() // TODO handle error
	b := &Block{
		SystemIdentifier:    p.configuration.SystemIdentifier,
		TxSystemBlockNumber: height + 1,
		PreviousBlockHash:   latestBlock.Hash(p.configuration.HashAlgorithm),
		Transactions:        transactions,
		UnicityCertificate:  ucr,
	}
	// TODO ensure block hash equals to IR hash
	_ = p.blockStore.Add(b) // TODO handle error
}

func (p *Partition) handleT1TimeoutEvent() {
	if p.leaderSelector.IsCurrentNodeLeader() {
		p.sendProposal()
	}
	p.leaderSelector.UpdateLeader(nil)
}

func (p *Partition) sendProposal() {
	logger.Info("Sending PC1-O event")
	systemIdentifier := p.configuration.SystemIdentifier
	nodeId := p.leaderSelector.SelfID()
	p.eventbus.Submit(TopicPC10, &PC1OEvent{
		SystemIdentifier:         systemIdentifier,
		NodeIdentifier:           nodeId,
		UnicityCertificateRecord: p.luc,
		Transactions:             p.proposal,
	})
	prevHash := p.luc.InputRecord.Hash
	stateRoot, summary := p.transactionSystem.RCompl()
	p.pr = &pendingBlockProposal{
		lucRoundNumber: p.luc.UnicityCertificate.RootChainBlockNumber,
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
	hasher.Write(p.configuration.SystemIdentifier)
	hasher.Write(transaction.Uint64ToBytes(blockNr))
	hasher.Write(prevBlockHash)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	blockHash := hasher.Sum(nil)

	p.proposal = []*transaction.Transaction{}

	logger.Info("Sending P1 event")
	p.eventbus.Submit(TopicP1, P1Event{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   nodeId,
		lucRoundNumber:   p.pr.lucRoundNumber,
		inputRecord: &certificates.InputRecord{
			PreviousHash: prevHash,
			Hash:         stateRoot,
			BlockHash:    blockHash,
			SummaryValue: summary.Bytes(),
		},
	})
}

func (p *Partition) resetTimer() {
	// stop timer before resetting it because "Reset" should be invoked only on stopped or expired timers with drained
	// channels.
	stopTimer(p.t1)
	p.t1.Reset(p.configuration.T1Timeout)
}

func stopTimer(t *time.Timer) {
	if !t.Stop() {
		// TODO drain channel. call "<-t.C" from the main event loop (AB-119)
		//<-t.C
	}
}
