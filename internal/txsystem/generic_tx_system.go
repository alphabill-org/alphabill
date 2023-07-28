package txsystem

import (
	"crypto"
	"fmt"
	"reflect"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

const PayloadTypePruneStates = "pruneStates"

var _ TransactionSystem = (*GenericTxSystem)(nil)

// SystemDescriptions is map of system description records indexed by System Identifiers
type SystemDescriptions map[string]*genesis.SystemDescriptionRecord

type Module interface {
	TxExecutors() map[string]TxExecutor
	GenericTransactionValidator() GenericTransactionValidator
}

type GenericTxSystem struct {
	systemIdentifier    []byte
	hashAlgorithm       crypto.Hash
	state               *state.State
	logPruner           *state.LogPruner
	currentBlockNumber  uint64
	systemGeneratedTxs  map[string]bool
	executors           TxExecutors
	genericTxValidators []GenericTransactionValidator
	beginBlockFunctions []TxEmitter
	endBlockFunctions   []TxEmitter
	sysGenTxHandler     OnTransactionsFunc
}

type (
	TxEmitter func(blockNumber uint64) ([]*types.TransactionRecord, error)
)

func NewGenericTxSystem(modules []Module, opts ...Option) (*GenericTxSystem, error) {
	txs := &GenericTxSystem{
		executors:           make(map[string]TxExecutor),
		genericTxValidators: []GenericTransactionValidator{},
	}

	opts = append(opts, WithSystemGeneratedTxTypes(PayloadTypePruneStates))
	opts = append(opts, WithBeginBlockFunctions(txs.generatePruneStatesTx))

	options := DefaultOptions()

	for _, option := range opts {
		option(options)
	}

	txs.systemIdentifier = options.systemIdentifier
	txs.hashAlgorithm = options.hashAlgorithm
	txs.state = options.state
	txs.logPruner = state.NewLogPruner(options.state)
	txs.beginBlockFunctions = options.beginBlockFunctions
	txs.endBlockFunctions = options.endBlockFunctions
	txs.systemGeneratedTxs = options.systemGeneratedTxs

	for _, module := range modules {
		validator := module.GenericTransactionValidator()
		if validator != nil {
			var add = true
			for _, txValidator := range txs.genericTxValidators {
				if reflect.ValueOf(txValidator).Pointer() == reflect.ValueOf(validator).Pointer() {
					add = false
					break
				}
			}
			if add {
				txs.genericTxValidators = append(txs.genericTxValidators, validator)
			}
		}

		executors := module.TxExecutors()
		for k, executor := range executors {
			txs.executors[k] = executor
		}
	}
	txs.executors[PayloadTypePruneStates] = pruneExecutorFunc(txs.logPruner)
	return txs, nil
}

func (m *GenericTxSystem) SetSystemGeneratedTxHandler(handler OnTransactionsFunc) {
	m.sysGenTxHandler = handler
}

func (m *GenericTxSystem) GetState() *state.State {
	return m.state
}

func (m *GenericTxSystem) CurrentBlockNumber() uint64 {
	return m.currentBlockNumber
}

func (m *GenericTxSystem) StateSummary() (State, error) {
	if !m.state.IsCommitted() {
		return nil, ErrStateContainsUncommittedChanges
	}
	return m.getState()
}

func (m *GenericTxSystem) getState() (State, error) {
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
	for _, function := range m.beginBlockFunctions {
		txs, err := function(blockNr)
		if err != nil {
			return fmt.Errorf("begin block function call failed: %w", err)
		}
		callback := m.sysGenTxHandler
		if callback != nil {
			if err = callback(blockNr, txs...); err != nil {
				return err
			}
		}
	}
	m.currentBlockNumber = blockNr
	return nil
}

func (m *GenericTxSystem) Execute(tx *types.TransactionOrder) (sm *types.ServerMetadata, err error) {
	if m.systemGeneratedTxs[tx.PayloadType()] {
		return m.executors.Execute(tx, m.currentBlockNumber)
	}

	u, _ := m.state.GetUnit(tx.UnitID(), false)
	ctx := &TxValidationContext{
		Tx:               tx,
		Unit:             u,
		SystemIdentifier: m.systemIdentifier,
		BlockNumber:      m.currentBlockNumber,
	}
	for _, validator := range m.genericTxValidators {
		if err = validator(ctx); err != nil {
			return nil, fmt.Errorf("invalid transaction: %w", err)
		}
	}

	m.state.Savepoint()
	defer func() {
		if err != nil {
			// transaction execution failed. revert every change made by the transaction order
			m.state.RollbackSavepoint()
			return
		}
		trx := &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   sm,
		}
		targets := sm.TargetUnits
		// Handle fees! NB! The "transfer to fee credit" and "reclaim fee credit" transactions in the money partition
		// and the "add fee credit" and "close free credit" transactions in all application partitions are special
		// cases: fees are handled intrinsically in those transactions.
		if sm.ActualFee > 0 && !transactions.IsFeeCreditTx(tx) {
			feeCreditRecordID := tx.GetClientFeeCreditRecordID()
			if err = m.state.Apply(unit.DecrCredit(feeCreditRecordID, sm.ActualFee)); err != nil {
				m.state.RollbackSavepoint()
				return
			}
			targets = append(targets, feeCreditRecordID)
		}
		for _, targetID := range targets {
			// add log for each target unit
			unitLogSize, err := m.state.AddUnitLog(targetID, trx.Hash(m.hashAlgorithm))
			if err != nil {
				m.state.RollbackSavepoint()
				return
			}
			if unitLogSize > 1 {
				m.logPruner.Add(m.currentBlockNumber, targetID)
			}
		}

		// transaction execution succeeded
		m.state.ReleaseSavepoint()
	}()
	// execute transaction
	sm, err = m.executors.Execute(tx, m.currentBlockNumber)
	if err != nil {
		return nil, err
	}

	return sm, err
}

func (m *GenericTxSystem) EndBlock() (State, error) {
	for _, function := range m.endBlockFunctions {
		txs, err := function(m.currentBlockNumber)
		if err != nil {
			return nil, fmt.Errorf("end block function call failed: %w", err)
		}
		callback := m.sysGenTxHandler
		if callback != nil {
			if err = callback(m.currentBlockNumber, txs...); err != nil {
				return nil, err
			}
		}
	}
	return m.getState()
}

func (m *GenericTxSystem) Revert() {
	m.logPruner.Remove(m.currentBlockNumber)
	m.state.Revert()
}

func (m *GenericTxSystem) Commit() error {
	m.logPruner.Remove(m.currentBlockNumber - 1)
	return m.state.Commit()
}

func pruneExecutorFunc(pruner *state.LogPruner) ExecuteFunc {
	return func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
		return &types.ServerMetadata{ActualFee: 0}, pruner.Prune(currentBlockNr - 1)
	}
}

func (m *GenericTxSystem) generatePruneStatesTx(blockNumber uint64) ([]*types.TransactionRecord, error) {
	if m.logPruner.Count(blockNumber-1) == 0 {
		return nil, nil
	}
	return []*types.TransactionRecord{
		{
			TransactionOrder: &types.TransactionOrder{
				Payload: &types.Payload{
					SystemID:       m.systemIdentifier,
					Type:           PayloadTypePruneStates,
					ClientMetadata: &types.ClientMetadata{Timeout: blockNumber + 1},
				},
			},
			ServerMetadata: &types.ServerMetadata{ActualFee: 0},
		},
	}, nil
}
