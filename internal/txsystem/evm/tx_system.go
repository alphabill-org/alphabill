package evm

import (
	"crypto"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type TxSystem struct {
	systemIdentifier    []byte
	hashAlgorithm       crypto.Hash
	state               *state.State
	logPruner           *state.LogPruner
	currentBlockNumber  uint64
	executors           txsystem.TxExecutors
	genericTxValidators []txsystem.GenericTransactionValidator
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
	roundCommitted      bool
	log                 *slog.Logger
}

func NewEVMTxSystem(systemIdentifier []byte, log *slog.Logger, opts ...Option) (*TxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	if options.state == nil {
		return nil, errors.New("evm tx system init failed, state tree is nil")
	}
	/*	if options.blockDB == nil {
		return nil, errors.New("evm tx system init failed, block DB is nil")
	}*/
	evm, err := NewEVMModule(systemIdentifier, options, log)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM module: %w", err)
	}
	fees, err := newFeeModule(systemIdentifier, options, log)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM fee module: %w", err)
	}
	txs := &TxSystem{
		systemIdentifier:    systemIdentifier,
		hashAlgorithm:       options.hashAlgorithm,
		state:               options.state,
		logPruner:           state.NewLogPruner(options.state),
		beginBlockFunctions: evm.StartBlockFunc(options.blockGasLimit),
		endBlockFunctions:   nil,
		executors:           make(map[string]txsystem.TxExecutor),
		genericTxValidators: []txsystem.GenericTransactionValidator{evm.GenericTransactionValidator(), fees.GenericTransactionValidator()},
		log:                 log,
	}
	txs.beginBlockFunctions = append(txs.beginBlockFunctions, txs.pruneLogs)
	executors := evm.TxExecutors()
	for k, executor := range executors {
		txs.executors[k] = executor
	}
	executors = fees.TxExecutors()
	for k, executor := range executors {
		txs.executors[k] = executor
	}
	return txs, nil
}

func (m *TxSystem) GetState() *state.State {
	return m.state
}

func (m *TxSystem) CurrentBlockNumber() uint64 {
	return m.currentBlockNumber
}

func (m *TxSystem) StateStorage() *state.State {
	return m.state
}

func (m *TxSystem) StateSummary() (txsystem.State, error) {
	if !m.state.IsCommitted() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return m.getState()
}

func (m *TxSystem) getState() (txsystem.State, error) {
	sv, hash, err := m.state.CalculateRoot()
	if err != nil {
		return nil, err
	}
	if hash == nil {
		return txsystem.NewStateSummary(make([]byte, m.hashAlgorithm.Size()), util.Uint64ToBytes(sv)), nil
	}
	return txsystem.NewStateSummary(hash, util.Uint64ToBytes(sv)), nil
}

func (m *TxSystem) BeginBlock(blockNr uint64) error {
	m.currentBlockNumber = blockNr
	m.roundCommitted = false
	for _, function := range m.beginBlockFunctions {
		if err := function(blockNr); err != nil {
			return fmt.Errorf("begin block function call failed: %w", err)
		}
	}
	return nil
}

func (m *TxSystem) pruneLogs(blockNr uint64) error {
	if err := m.logPruner.Prune(blockNr - 1); err != nil {
		return fmt.Errorf("unable to prune state: %w", err)
	}
	return nil
}

func (m *TxSystem) Execute(tx *types.TransactionOrder) (sm *types.ServerMetadata, err error) {
	u, _ := m.state.GetUnit(tx.UnitID(), false)
	ctx := &txsystem.TxValidationContext{
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

	savepointID := m.state.Savepoint()
	defer func() {
		if err != nil {
			// transaction execution failed. revert every change made by the transaction order
			m.state.RollbackToSavepoint(savepointID)
			return
		}
		trx := &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   sm,
		}
		for _, targetID := range sm.TargetUnits {
			// add log for each target unit
			unitLogSize, err := m.state.AddUnitLog(targetID, trx.Hash(m.hashAlgorithm))
			if err != nil {
				m.state.RollbackToSavepoint(savepointID)
				return
			}
			if unitLogSize > 1 {
				m.logPruner.Add(m.currentBlockNumber, targetID)
			}
		}

		// transaction execution succeeded
		m.state.ReleaseToSavepoint(savepointID)
	}()
	// execute transaction
	m.log.Debug(fmt.Sprintf("execute %s", tx.PayloadType()), logger.UnitID(tx.UnitID()), logger.Data(tx), logger.Round(m.currentBlockNumber))
	sm, err = m.executors.Execute(tx, m.currentBlockNumber)
	if err != nil {
		return nil, err
	}

	return sm, err
}

func (m *TxSystem) EndBlock() (txsystem.State, error) {
	for _, function := range m.endBlockFunctions {
		if err := function(m.currentBlockNumber); err != nil {
			return nil, fmt.Errorf("end block function call failed: %w", err)
		}
	}
	return m.getState()
}

func (m *TxSystem) Revert() {
	if m.roundCommitted {
		return
	}
	m.logPruner.Remove(m.currentBlockNumber)
	m.state.Revert()
}

func (m *TxSystem) Commit() error {
	m.logPruner.Remove(m.currentBlockNumber - 1)
	err := m.state.Commit()
	if err == nil {
		m.roundCommitted = true
	}
	return err
}
