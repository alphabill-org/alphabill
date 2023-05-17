package txsystem

import (
	"crypto"
	"fmt"
	"reflect"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

var _ TransactionSystem = &GenericTxSystem{}

// SystemDescriptions is map of system description records indexed by System Identifiers
type SystemDescriptions map[string]*genesis.SystemDescriptionRecord

type Module interface {
	TxExecutors() map[string]TxExecutor
	GenericTransactionValidator() GenericTransactionValidator
	//	TxConverter() TxConverters
}

type GenericTxSystem struct {
	systemIdentifier   []byte
	hashAlgorithm      crypto.Hash
	state              *rma.Tree
	currentBlockNumber uint64

	executors TxExecutors
	//	txConverters        TxConverters
	genericTxValidators []GenericTransactionValidator
	beginBlockFunctions []func(blockNumber uint64)
	endBlockFunctions   []func(blockNumber uint64) error
}

func NewGenericTxSystem(modules []Module, opts ...Option) (*GenericTxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	// initial changes made may require tree recompute
	// get root hash will trigger recompute if needed
	// todo: AB-576 remove both lines after new AVL tree is added
	options.state.GetRootHash()
	options.state.Commit()
	txs := &GenericTxSystem{
		systemIdentifier:    options.systemIdentifier,
		hashAlgorithm:       options.hashAlgorithm,
		state:               options.state,
		beginBlockFunctions: options.beginBlockFunctions,
		endBlockFunctions:   options.endBlockFunctions,
		executors:           make(map[string]TxExecutor),
		genericTxValidators: []GenericTransactionValidator{},
	}
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
	return txs, nil
}

func (m *GenericTxSystem) GetState() *rma.Tree {
	return m.state
}

func (m *GenericTxSystem) CurrentBlockNumber() uint64 {
	return m.currentBlockNumber
}

func (m *GenericTxSystem) StateSummary() (State, error) {
	if m.state.ContainsUncommittedChanges() {
		return nil, ErrStateContainsUncommittedChanges
	}
	return m.getState()
}

func (m *GenericTxSystem) getState() (State, error) {
	sv := m.state.TotalValue()
	if sv == nil {
		sv = rma.Uint64SummaryValue(0)
	}
	hash := m.state.GetRootHash()
	if hash == nil {
		hash = make([]byte, m.hashAlgorithm.Size())
	}
	return NewStateSummary(hash, sv.Bytes()), nil
}

func (m *GenericTxSystem) BeginBlock(blockNr uint64) {
	for _, function := range m.beginBlockFunctions {
		function(blockNr)
	}
	m.currentBlockNumber = blockNr
}

func (m *GenericTxSystem) Execute(tx *types.TransactionOrder) (*types.ServerMetadata, error) {
	u, _ := m.state.GetUnit(util.BytesToUint256(tx.UnitID()))
	ctx := &TxValidationContext{
		Tx:               tx,
		Unit:             u,
		SystemIdentifier: m.systemIdentifier,
		BlockNumber:      m.currentBlockNumber,
	}
	for _, validator := range m.genericTxValidators {
		if err := validator(ctx); err != nil {
			return nil, fmt.Errorf("invalid transaction: %w", err)
		}
	}

	return m.executors.Execute(tx, m.currentBlockNumber)
}

func (m *GenericTxSystem) EndBlock() (State, error) {
	for _, function := range m.endBlockFunctions {
		if err := function(m.currentBlockNumber); err != nil {
			return nil, fmt.Errorf("end block function call failed: %w", err)
		}
	}
	return m.getState()
}

func (m *GenericTxSystem) Revert() {
	m.state.Revert()
}

func (m *GenericTxSystem) Commit() {
	m.state.Commit()
}
