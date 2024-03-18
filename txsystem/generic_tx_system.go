package txsystem

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/alphabill-org/alphabill/predicates"
	"go.opentelemetry.io/otel/metric"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

var _ TransactionSystem = (*GenericTxSystem)(nil)

type Module interface {
	TxExecutors() map[string]ExecuteFunc
}

type GenericTxSystem struct {
	systemIdentifier      types.SystemID
	hashAlgorithm         crypto.Hash
	state                 *state.State
	currentBlockNumber    uint64
	executors             TxExecutors
	checkFeeCreditBalance func(tx *types.TransactionOrder) error
	beginBlockFunctions   []func(blockNumber uint64) error
	endBlockFunctions     []func(blockNumber uint64) error
	roundCommitted        bool
	log                   *slog.Logger
	pr                    predicates.PredicateRunner
}

type FeeCreditBalanceValidator func(tx *types.TransactionOrder) error

type Observability interface {
	Meter(name string, opts ...metric.MeterOption) metric.Meter
	Logger() *slog.Logger
}

func NewGenericTxSystem(systemID types.SystemID, feeChecker FeeCreditBalanceValidator, modules []Module, observe Observability, opts ...Option) (*GenericTxSystem, error) {
	if systemID == 0 {
		return nil, errors.New("system ID must be assigned")
	}
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	txs := &GenericTxSystem{
		systemIdentifier:      systemID,
		hashAlgorithm:         options.hashAlgorithm,
		state:                 options.state,
		beginBlockFunctions:   options.beginBlockFunctions,
		endBlockFunctions:     options.endBlockFunctions,
		executors:             make(TxExecutors),
		checkFeeCreditBalance: feeChecker,
		log:                   observe.Logger(),
		pr:                    options.predicateRunner,
	}
	txs.beginBlockFunctions = append(txs.beginBlockFunctions, txs.pruneState)
	modules = append(modules, NewIdentityModule(txs, txs.state))

	for _, module := range modules {
		if err := txs.executors.Add(module.TxExecutors()); err != nil {
			return nil, fmt.Errorf("registering tx executors: %w", err)
		}
	}

	if err := txs.initMetrics(observe.Meter("txsystem")); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}

	return txs, nil
}

func (m *GenericTxSystem) StateSummary() (StateSummary, error) {
	if !m.state.IsCommitted() {
		return nil, ErrStateContainsUncommittedChanges
	}
	return m.getStateSummary()
}

func (m *GenericTxSystem) getStateSummary() (StateSummary, error) {
	sv, hash, err := m.state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	if hash == nil {
		return NewStateSummary(make([]byte, m.hashAlgorithm.Size()), util.Uint64ToBytes(sv)), nil
	}
	return NewStateSummary(hash, util.Uint64ToBytes(sv)), nil
}

func (m *GenericTxSystem) BeginBlock(blockNr uint64) error {
	m.currentBlockNumber = blockNr
	m.roundCommitted = false
	for _, function := range m.beginBlockFunctions {
		if err := function(blockNr); err != nil {
			return fmt.Errorf("begin block function call failed: %w", err)
		}
	}
	return nil
}

func (m *GenericTxSystem) pruneState(blockNr uint64) error {
	return m.state.Prune()
}

func (m *GenericTxSystem) Execute(tx *types.TransactionOrder) (sm *types.ServerMetadata, rErr error) {
	return m.doExecute(tx, false)
}

func (m *GenericTxSystem) doExecute(tx *types.TransactionOrder, stateLockReleased bool) (sm *types.ServerMetadata, rErr error) {
	if err := m.validateGenericTransaction(tx); err != nil {
		return nil, fmt.Errorf("invalid transaction: %w", err)
	}

	ctx := &TxExecutionContext{
		CurrentBlockNr:    m.currentBlockNumber,
		StateLockReleased: stateLockReleased,
	}

	savepointID := m.state.Savepoint()
	defer func() {
		if rErr != nil {
			// transaction execution failed. revert every change made by the transaction order
			m.state.RollbackToSavepoint(savepointID)
			return
		}

		// Handle fees! NB! The "transfer to fee credit" and "reclaim fee credit" transactions in the money partition
		// and the "lock fee credit", "unlock fee credit", "add fee credit" and "close free credit" transactions in all
		// application partitions are special cases: fees are handled intrinsically in those transactions.
		if sm.ActualFee > 0 && !transactions.IsFeeCreditTx(tx) {
			feeCreditRecordID := tx.GetClientFeeCreditRecordID()
			if err := m.state.Apply(unit.DecrCredit(feeCreditRecordID, sm.ActualFee)); err != nil {
				m.state.RollbackToSavepoint(savepointID)
				rErr = fmt.Errorf("handling tx fee: %w", err)
				return
			}
			sm.TargetUnits = append(sm.TargetUnits, feeCreditRecordID)
		}
		trx := &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   sm,
		}
		for _, targetID := range sm.TargetUnits {
			// add log for each target unit
			if err := m.state.AddUnitLog(targetID, trx.Hash(m.hashAlgorithm)); err != nil {
				m.state.RollbackToSavepoint(savepointID)
				rErr = fmt.Errorf("adding unit log: %w", err)
				return
			}
		}

		// transaction execution succeeded
		m.state.ReleaseToSavepoint(savepointID)
	}()

	// check state lock and release it if possible
	rErr = m.validateUnitStateLock(tx)
	if rErr != nil {
		return nil, rErr
	}

	// proceed with the transaction execution
	m.log.Debug(fmt.Sprintf("execute %s", tx.PayloadType()), logger.UnitID(tx.UnitID()), logger.Data(tx), logger.Round(m.currentBlockNumber))
	sm, rErr = m.executors.Execute(tx, ctx)
	if rErr != nil {
		return nil, rErr
	}
	return sm, nil
}

/*
validateGenericTransaction does the tx validation common to all tx systems.

See Yellowpaper chapter 4.6 "Valid Transaction Orders".
The (final) step "ψτ(P,S) – type-specific validity condition holds" must be
implemented by the tx handler.
*/
func (m *GenericTxSystem) validateGenericTransaction(tx *types.TransactionOrder) error {
	// 1. P.α = S.α – transaction is sent to this system
	if m.systemIdentifier != tx.SystemID() {
		return ErrInvalidSystemIdentifier
	}

	// 2. fSH(P.ι)=S.σ–target unit is in this shard

	// 3. n < T0 – transaction has not expired
	if m.currentBlockNumber >= tx.Timeout() {
		return ErrTransactionExpired
	}

	// 4. N[ι] = ⊥ ∨ VerifyOwner(N[ι].φ, P, P.s) = 1 – owner proof verifies correctly.
	// Yellowpaper currently suggests to check owner proof here. However it requires
	// knowledge about whether it's ok that the unit is not part of current
	// state - ie create token type or mint token transactions. So we do the
	// owner proof verification in the tx handler.

	// the checkFeeCreditBalance must verify the conditions 5 to 9 listed in the
	// Yellowpaper "Valid Transaction Orders" chapter.
	if err := m.checkFeeCreditBalance(tx); err != nil {
		return fmt.Errorf("fee credit balance check: %w", err)
	}

	return nil
}

func (m *GenericTxSystem) State() StateReader {
	return m.state.Clone()
}

func (m *GenericTxSystem) EndBlock() (StateSummary, error) {
	for _, function := range m.endBlockFunctions {
		if err := function(m.currentBlockNumber); err != nil {
			return nil, fmt.Errorf("end block function call failed: %w", err)
		}
	}
	return m.getStateSummary()
}

func (m *GenericTxSystem) Revert() {
	if m.roundCommitted {
		return
	}
	m.state.Revert()
}

func (m *GenericTxSystem) Commit(uc *types.UnicityCertificate) error {
	err := m.state.Commit(uc)
	if err == nil {
		m.roundCommitted = true
	}
	return err
}

func (m *GenericTxSystem) CommittedUC() *types.UnicityCertificate {
	return m.state.CommittedUC()
}

func (m *GenericTxSystem) SerializeState(writer io.Writer, committed bool) error {
	return m.state.Serialize(writer, committed)
}

func (m *GenericTxSystem) initMetrics(mtr metric.Meter) error {
	if _, err := mtr.Int64ObservableUpDownCounter(
		"unit.count",
		metric.WithDescription(`Number of units in the state.`),
		metric.WithUnit("{unit}"),
		metric.WithInt64Callback(func(ctx context.Context, io metric.Int64Observer) error {
			snc := state.NewStateNodeCounter()
			m.state.Traverse(snc)
			io.Observe(int64(snc.NodeCount()))
			return nil
		}),
	); err != nil {
		return fmt.Errorf("creating state unit counter: %w", err)
	}

	return nil
}
