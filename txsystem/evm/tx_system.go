package evm

import (
	"crypto"
	"errors"
	"fmt"
	"log/slog"
	"math"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

type Observability interface {
	Tracer(name string, options ...trace.TracerOption) trace.Tracer
	TracerProvider() trace.TracerProvider

	Meter(name string, opts ...metric.MeterOption) metric.Meter
	PrometheusRegisterer() prometheus.Registerer

	Logger() *slog.Logger
	RoundLogger(curRound func() uint64) *slog.Logger

	Shutdown() error
}

type genericTransactionValidator func(ctx *TxValidationContext) error

type TxValidationContext struct {
	Tx          *types.TransactionOrder
	state       *state.State
	NetworkID   types.NetworkID
	PartitionID types.PartitionID
	BlockNumber uint64
	CustomData  []byte
	exArgument  func() ([]byte, error)
	exeType     txtypes.ExecutionType
}

type TxSystem struct {
	networkID           types.NetworkID
	partitionID         types.PartitionID
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

func NewEVMTxSystem(networkID types.NetworkID, partitionID types.PartitionID, observe Observability, opts ...Option) (*TxSystem, error) {
	options, err := defaultOptions(observe)
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
	txs := &TxSystem{
		networkID:         networkID,
		partitionID:       partitionID,
		hashAlgorithm:     options.hashAlgorithm,
		state:             options.state,
		endBlockFunctions: nil,
		executors:         make(txtypes.TxExecutors),
	}
	txs.log = observe.RoundLogger(txs.CurrentBlockNumber)
	observe = observability.WithLogger(observe, txs.log)
	evm, err := NewEVMModule(partitionID, options, txs.log)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM module: %w", err)
	}
	fees, err := newFeeModule(partitionID, options, observe)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM fee module: %w", err)
	}
	txs.genericTxValidators = []genericTransactionValidator{evm.GenericTransactionValidator(), fees.GenericTransactionValidator()}
	txs.beginBlockFunctions = append(evm.StartBlockFunc(options.blockGasLimit), txs.pruneState)
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
	committed, err := m.state.IsCommitted()
	if err != nil {
		return 0, fmt.Errorf("unable to check if state is committed: %w", err)
	}
	if !committed {
		return 0, txsystem.ErrStateContainsUncommittedChanges
	}
	return m.state.Size()
}

func (m *TxSystem) StateSummary() (txsystem.StateSummary, error) {
	committed, err := m.state.IsCommitted()
	if err != nil {
		return nil, fmt.Errorf("unable to check if state is committed: %w", err)
	}
	if !committed {
		return nil, txsystem.ErrStateContainsUncommittedChanges
	}
	return m.getState()
}

func (m *TxSystem) getState() (txsystem.StateSummary, error) {
	sv, hash, err := m.state.CalculateRoot()
	if err != nil {
		return nil, err
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

func (m *TxSystem) Execute(tx *types.TransactionOrder) (txr *types.TransactionRecord, err error) {
	exeCtx := &TxValidationContext{
		Tx:          tx,
		state:       m.state,
		NetworkID:   m.networkID,
		PartitionID: m.partitionID,
		BlockNumber: m.currentRoundNumber,
	}
	for _, validator := range m.genericTxValidators {
		if err = validator(exeCtx); err != nil {
			return nil, fmt.Errorf("invalid transaction: %w", err)
		}
	}
	txBytes, err := tx.MarshalCBOR()
	if err != nil {
		return nil, fmt.Errorf("transaction order serialization error: %w", err)
	}
	txr = &types.TransactionRecord{
		Version:          1,
		TransactionOrder: txBytes,
	}
	savepointID, err := m.state.Savepoint()
	if err != nil {
		return nil, fmt.Errorf("savepoint creation failed: %w", err)
	}
	defer func() {
		if err != nil {
			// transaction execution failed. revert every change made by the transaction order
			m.state.RollbackToSavepoint(savepointID)
			return
		}

		for _, targetID := range txr.ServerMetadata.TargetUnits {
			// add log for each target unit
			txrHash, err := txr.Hash(m.hashAlgorithm)
			if err != nil {
				m.state.RollbackToSavepoint(savepointID)
				return
			}
			if err := m.state.AddUnitLog(targetID, txrHash); err != nil {
				m.state.RollbackToSavepoint(savepointID)
				return
			}
		}

		// transaction execution succeeded
		m.state.ReleaseToSavepoint(savepointID)
	}()
	// execute transaction
	m.log.Debug(fmt.Sprintf("execute %d", tx.Type), logger.UnitID(tx.UnitID), logger.Data(tx))
	txr.ServerMetadata, err = m.executors.ValidateAndExecute(tx, exeCtx)
	if err != nil {
		return nil, err
	}
	return txr, err
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

func (m *TxSystem) IsPermissionedMode() bool {
	return false
}

func (m *TxSystem) IsFeelessMode() bool {
	return false
}

func (m *TxSystem) TypeID() types.PartitionTypeID {
	return evm.PartitionTypeID
}

func (vc *TxValidationContext) GetUnit(id types.UnitID, committed bool) (state.Unit, error) {
	return vc.state.GetUnit(id, committed)
}

func (vc *TxValidationContext) CommittedUC() *types.UnicityCertificate {
	return vc.state.CommittedUC()
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

func (vc *TxValidationContext) ExtraArgument() ([]byte, error) {
	if vc.exArgument == nil {
		return nil, errors.New("extra argument callback not assigned")
	}
	return vc.exArgument()
}

func (vc *TxValidationContext) WithExArg(f func() ([]byte, error)) txtypes.ExecutionContext {
	vc.exArgument = f
	return vc
}

func (vc *TxValidationContext) ExecutionType() txtypes.ExecutionType {
	return vc.exeType
}

func (vc *TxValidationContext) SetExecutionType(exeType txtypes.ExecutionType) {
	vc.exeType = exeType
}
