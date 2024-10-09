package evm

import (
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

type genericTransactionValidator func(ctx *TxValidationContext) error

type TxValidationContext struct {
	Tx          *types.TransactionOrder
	state       *state.State
	NetworkID   types.NetworkID
	SystemID    types.SystemID
	BlockNumber uint64
	CustomData  []byte
}

type TxSystem struct {
	networkID           types.NetworkID
	systemID            types.SystemID
	hashAlgorithm       crypto.Hash
	state               *state.State
	currentRoundNumber  uint64
	executors           txtypes.TxExecutors
	genericTxValidators []genericTransactionValidator
	beginBlockFunctions []func(blockNumber uint64) error
	endBlockFunctions   []func(blockNumber uint64) error
	roundCommitted      bool
	log                 *slog.Logger
}

func NewEVMTxSystem(networkID types.NetworkID, systemID types.SystemID, log *slog.Logger, opts ...Option) (*TxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}
	if options.state == nil {
		return nil, errors.New("evm transaction system init failed, state tree is nil")
	}
	/*	if options.blockDB == nil {
		return nil, errors.New("evm tx system init failed, block DB is nil")
	}*/
	evm, err := NewEVMModule(systemID, options, log)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM module: %w", err)
	}
	fees, err := newFeeModule(systemID, options, log)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM fee module: %w", err)
	}
	txs := &TxSystem{
		networkID:           networkID,
		systemID:            systemID,
		hashAlgorithm:       options.hashAlgorithm,
		state:               options.state,
		beginBlockFunctions: evm.StartBlockFunc(options.blockGasLimit),
		endBlockFunctions:   nil,
		executors:           make(txtypes.TxExecutors),
		genericTxValidators: []genericTransactionValidator{evm.GenericTransactionValidator(), fees.GenericTransactionValidator()},
		log:                 log,
	}
	txs.beginBlockFunctions = append(txs.beginBlockFunctions, txs.pruneState)
	if err := txs.executors.Add(evm.TxHandlers()); err != nil {
		return nil, fmt.Errorf("registering EVM executors: %w", err)
	}
	if err := txs.executors.Add(fees.TxHandlers()); err != nil {
		return nil, fmt.Errorf("registering fee executors: %w", err)
	}

	return txs, nil
}

func (m *TxSystem) CurrentBlockNumber() uint64 {
	return m.currentRoundNumber
}

func (m *TxSystem) State() txsystem.StateReader {
	return m.state.Clone()
}

func (m *TxSystem) StateSize() (uint64, error) {
	if !m.state.IsCommitted() {
		return 0, txsystem.ErrStateContainsUncommittedChanges
	}
	return m.state.Size()
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

func (m *TxSystem) BeginBlock(roundNo uint64) error {
	m.currentRoundNumber = roundNo
	m.roundCommitted = false
	for _, function := range m.beginBlockFunctions {
		if err := function(roundNo); err != nil {
			return fmt.Errorf("begin block function call failed: %w", err)
		}
	}
	return nil
}

func (m *TxSystem) pruneState(roundNo uint64) error {
	return m.state.Prune()
}

func (m *TxSystem) Execute(tx *types.TransactionOrder) (sm *types.ServerMetadata, err error) {
	exeCtx := &TxValidationContext{
		Tx:          tx,
		state:       m.state,
		NetworkID:   m.networkID,
		SystemID:    m.systemID,
		BlockNumber: m.currentRoundNumber,
	}
	for _, validator := range m.genericTxValidators {
		if err = validator(exeCtx); err != nil {
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
	m.log.Debug(fmt.Sprintf("execute %d", tx.Type), logger.UnitID(tx.UnitID), logger.Data(tx), logger.Round(m.currentRoundNumber))
	sm, err = m.executors.ValidateAndExecute(tx, exeCtx)
	if err != nil {
		return nil, err
	}
	return sm, err
}

func (m *TxSystem) EndBlock() (txsystem.StateSummary, error) {
	for _, function := range m.endBlockFunctions {
		if err := function(m.currentRoundNumber); err != nil {
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

func (vc *TxValidationContext) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return vc.state.GetUnit(id, committed)
}

func (vc *TxValidationContext) CurrentRound() uint64 { return vc.BlockNumber }

func (vc *TxValidationContext) TrustBase(epoch uint64) (types.RootTrustBase, error) {
	return nil, fmt.Errorf("TxValidationContext.TrustBase not implemented")
}

func (vc *TxValidationContext) GasAvailable() uint64 {
	return math.MaxUint64
}

func (vc *TxValidationContext) SpendGas(gas uint64) error {
	return nil
}

func (vc *TxValidationContext) CalculateCost() uint64 { return 0 }

func (vc *TxValidationContext) TransactionOrder() (*types.TransactionOrder, error) { return vc.Tx, nil }

func (vc *TxValidationContext) GetData() []byte {
	return vc.CustomData
}

func (vc *TxValidationContext) SetData(data []byte) {
	vc.CustomData = data
}
