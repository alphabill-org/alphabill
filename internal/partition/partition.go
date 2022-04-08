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
	ErrTxSystemIsNil                    = errors.New("transaction system is nil")
	ErrEventBusIsNil                    = errors.New("event bus is nil")
	ErrPartitionConfigurationIsNil      = errors.New("configuration is nil")
	ErrLeaderSelectorIsNil              = errors.New("leader selector is nil")
	ErrCtxIsNil                         = errors.New("ctx is nil")
	ErrUnicityCertificateValidatorIsNil = errors.New("unicity certificate validator is nil")
	ErrTxValidatorIsNil                 = errors.New("tx validator is nil")
	ErrGenesisIsNil                     = errors.New("genesis is nil")
	ErrInvalidRootHash                  = errors.New("tx system root hash does not equal to genesis file hash")
	ErrInvalidSummaryValue              = errors.New("tx system summary value does not equal to genesis file summary value")
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
		transactionSystem                 TransactionSystem
		configuration                     *Configuration
		luc                               *certificates.UnicityCertificate
		proposal                          []*transaction.Transaction
		pr                                *pendingBlockProposal
		t1                                *time.Timer
		leaderSelector                    *LeaderSelector
		txValidator                       TxValidator
		unicityCertificateRecordValidator UnicityCertificateValidator
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
	ucValidator UnicityCertificateValidator,
	txValidator TxValidator,
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

	blockStorage := newInMemoryBlockStore()

	if err := partitionGenesis.IsValid(configuration.TrustBase, configuration.HashAlgorithm); err != nil {
		logger.Warning("Invalid partition genesis file: %v", err)
		return nil, errors.Wrap(err, "invalid root partition genesis file")
	}

	txGenesisRoot, txSummaryValue := txSystem.RCompl()
	genesisCertificate := partitionGenesis.Certificate
	genesisInputRecord := genesisCertificate.InputRecord
	if !bytes.Equal(genesisInputRecord.Hash, txGenesisRoot) {
		logger.Warning("tx system root hash does not equal to genesis file hash. "+
			"genesis hash: %X, txSystem hash: %X", genesisInputRecord.Hash, txGenesisRoot)
		return nil, ErrInvalidRootHash
	}

	if !bytes.Equal(genesisInputRecord.SummaryValue, txSummaryValue.Bytes()) {
		logger.Warning("tx system summary value does not equal to genesis file summary value. "+
			"Genesis SummaryValue: %X, TxSystem SummaryValue: %X", genesisInputRecord.SummaryValue, txSummaryValue.Bytes())
		return nil, ErrInvalidSummaryValue
	}

	p := &Partition{
		transactionSystem:                 txSystem,
		configuration:                     configuration,
		t1:                                t1,
		leaderSelector:                    leaderSelector,
		eventbus:                          eb,
		unicityCertificatesCh:             unicityCertificatesCh,
		transactionsCh:                    transactionsCh,
		unicityCertificateRecordValidator: ucValidator,
		txValidator:                       txValidator,
		blockStore:                        blockStorage,
	}
	p.ctx, p.ctxCancel = context.WithCancel(ctx)

	genesisBlock := &Block{
		SystemIdentifier:    p.configuration.GetSystemIdentifier(),
		TxSystemBlockNumber: 1,
		PreviousBlockHash:   nil,
		Transactions:        []*transaction.Transaction{},
		UnicityCertificate:  genesisCertificate,
	}
	err = p.blockStore.Add(genesisBlock)
	if err != nil {
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

func (p *Partition) startNewRound(uc *certificates.UnicityCertificate) {
	p.transactionSystem.RInit()
	p.proposal = nil
	p.pr = nil
	p.leaderSelector.UpdateLeader(uc.UnicitySeal)
	p.luc = uc
	p.resetTimer()
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
	logger.Debug("Transaction processed. ProposalSize: %v", len(p.proposal))
}

func (p *Partition) handleUnicityCertificateRecordEvent(event interface{}) {
	switch event.(type) {
	case UnicityCertificateEvent:
		p.handleUnicityCertificateRecord(event.(UnicityCertificateEvent).Certificate)
	default:
		logger.Warning("Invalid event: %v", event)
	}
}

func (p *Partition) handleUnicityCertificateRecord(uc *certificates.UnicityCertificate) {
	// TODO refactor and write tests after task AB-130 is done
	if uc == nil {
		logger.Warning("Invalid UnicityCertificateEvent. UC is nil")
		return
	}

	if err := p.unicityCertificateRecordValidator.Validate(uc); err != nil {
		logger.Warning("Invalid UnicityCertificate. Invalid system identifier. Expected: %X, got %X", p.configuration.GetSystemIdentifier(), uc.UnicityTreeCertificate.SystemIdentifier)
		return
	}

	// ensure(uc.Cuni.α = α)
	if !bytes.Equal(uc.UnicityTreeCertificate.SystemIdentifier, p.configuration.GetSystemIdentifier()) {
		logger.Warning("Invalid UnicityCertificate. Invalid system identifier. Expected: %X, got %X", p.configuration.GetSystemIdentifier(), uc.UnicityTreeCertificate.SystemIdentifier)
		return
	}
	// ensure(¬(uc.UC.nr = luc.UC.nr ∧ uc.IR.h , luc.IR.h))
	if uc.UnicitySeal.RootChainRoundNumber == p.luc.UnicitySeal.RootChainRoundNumber &&
		!bytes.Equal(uc.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Equivocating certificates. UC: %v,LUC: %v", uc, p.luc)
		return
	}
	// ensure(¬(uc.IR.h′ = luc.IR.h′ ∧ uc.IR.h , luc.IR.h))
	if bytes.Equal(uc.InputRecord.PreviousHash, p.luc.InputRecord.PreviousHash) &&
		!bytes.Equal(uc.InputRecord.Hash, p.luc.InputRecord.Hash) {
		logger.Warning("Equivocating certificates. UC: %v,LUC: %v", uc, p.luc)
		return
	}
	//ensure(uc.UC.nr > luc.UC.nr)
	if uc.UnicitySeal.RootChainRoundNumber < p.luc.UnicitySeal.RootChainRoundNumber {
		logger.Warning("Received UC is older than LUC. UC:  %v,LUC: %v", uc, p.luc)
		return
	}
	if p.pr == nil {
		hash, _ := p.transactionSystem.RCompl()
		if !bytes.Equal(uc.InputRecord.Hash, hash) {
			logger.Warning("Starting recovery. UC: %v,LUC: %v", uc, p.luc)
			p.status = recovering
			// TODO start recovery (AB-41)
			return
		}
	} else if bytes.Equal(uc.InputRecord.Hash, p.pr.stateHash) {
		p.finalizeBlock(p.pr.Transactions, uc)
	} else if bytes.Equal(uc.InputRecord.Hash, p.pr.lucIRHash) {
		logger.Warning("Reverting state tree. UC: %v,LUC: %v", uc, p.luc)
		p.transactionSystem.Revert()
	} else {
		logger.Warning("Reverting state tree. UC: %v,LUC: %v", uc, p.luc)
		p.transactionSystem.Revert()
		logger.Warning("Starting recovery. UC: %v,LUC: %v", uc, p.luc)
		p.status = recovering
		// TODO start recovery (AB-41)
	}
	p.startNewRound(uc)
}

func (p *Partition) finalizeBlock(transactions []*transaction.Transaction, uc *certificates.UnicityCertificate) {
	logger.Info("Finalizing block. TxCount: %v", len(transactions))
	height, _ := p.blockStore.Height()           // TODO handle error
	latestBlock, _ := p.blockStore.LatestBlock() // TODO handle error
	b := &Block{
		SystemIdentifier:    p.configuration.GetSystemIdentifier(),
		TxSystemBlockNumber: height + 1,
		PreviousBlockHash:   latestBlock.Hash(p.configuration.HashAlgorithm),
		Transactions:        transactions,
		UnicityCertificate:  uc,
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
	logger.Info("Sending PC1-O event. TxCount: %v", len(p.proposal))
	systemIdentifier := p.configuration.GetSystemIdentifier()
	nodeId := p.leaderSelector.SelfID()
	err := p.eventbus.Submit(TopicPC1O, &PC1OEvent{
		SystemIdentifier:         systemIdentifier,
		NodeIdentifier:           nodeId,
		UnicityCertificateRecord: p.luc,
		Transactions:             p.proposal,
	})
	if err != nil {
		logger.Warning("Failed to send PC1-0 event: %v", err)
		return
	}
	prevHash := p.luc.InputRecord.Hash
	stateRoot, summary := p.transactionSystem.RCompl()
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
	hasher.Write(transaction.Uint64ToBytes(blockNr))
	hasher.Write(prevBlockHash)
	// TODO continue implementing after task AB-129
	/*for _, tx := range b.Transactions {
		tx.AddToHasher(hasher)
	}*/
	blockHash := hasher.Sum(nil)

	p.proposal = []*transaction.Transaction{}

	logger.Info("Sending P1 event")
	err = p.eventbus.Submit(TopicP1, P1Event{
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
	if err != nil {
		logger.Warning("Failed to send P1 event: %v", err)
	}
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
