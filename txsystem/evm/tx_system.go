package evm

import (
	"crypto"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
)

type genericTransactionValidator func(ctx *TxValidationContext) error

type TxValidationContext struct {
	Tx               *types.TransactionOrder
	Unit             *state.Unit
	SystemIdentifier types.SystemID
	BlockNumber      uint64
}

type TxSystem struct {
	systemIdentifier    types.SystemID
	hashAlgorithm       crypto.Hash
	state               *state.State
	currentBlockNumber  uint64
	executors           txsystem.TxExecutors
	genericTxValidators []genericTransactionValidator
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
	roundCommitted      bool
	log                 *slog.Logger
}

func NewEVMTxSystem(systemIdentifier types.SystemID, log *slog.Logger, opts ...Option) (*TxSystem, error) {
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
		beginBlockFunctions: evm.StartBlockFunc(options.blockGasLimit),
		endBlockFunctions:   nil,
		executors:           make(map[string]txsystem.TxExecutor),
		genericTxValidators: []genericTransactionValidator{evm.GenericTransactionValidator(), fees.GenericTransactionValidator()},
		log:                 log,
	}
	txs.beginBlockFunctions = append(txs.beginBlockFunctions, txs.pruneState)
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

func (m *TxSystem) CurrentBlockNumber() uint64 {
	return m.currentBlockNumber
}

func (m *TxSystem) State() *state.State {
	return m.state.Clone()
}

func (m *TxSystem) StateSummary() (txsystem.StateSummary, error) {
	if !m.state.IsCommitted() {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return m.getState()
}

func (m *TxSystem) getState() (txsystem.StateSummary, error) {
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

func (m *TxSystem) pruneState(blockNr uint64) error {
	return m.state.Prune()
}

func (m *TxSystem) Execute(tx *types.TransactionOrder) (sm *types.ServerMetadata, err error) {
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
			err := m.state.AddUnitLog(targetID, trx.Hash(m.hashAlgorithm))
			if err != nil {
				m.state.RollbackToSavepoint(savepointID)
				return
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

func (m *TxSystem) EndBlock() (txsystem.StateSummary, error) {
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
	m.state.Revert()
}

func (m *TxSystem) Commit(uc *types.UnicityCertificate) error {
	err := m.state.Commit(uc)
	if err == nil {
		m.roundCommitted = true
	}
	return err
}

func (m *TxSystem) CommittedUC() *types.UnicityCertificate {
	return m.state.CommittedUC()
}

func (m *TxSystem) SerializeState(writer io.Writer, committed bool) error {
	header := &state.Header{
		SystemIdentifier: m.systemIdentifier,
	}
	return m.state.Serialize(writer, header, committed)
}
