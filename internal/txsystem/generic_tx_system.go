package txsystem

import (
	"crypto"
	"errors"
	"fmt"
	"reflect"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/holiman/uint256"
)

var _ TransactionSystem = &GenericTxSystem{}

// SystemDescriptions is map of system description records indexed by System Identifiers
type SystemDescriptions map[string]*genesis.SystemDescriptionRecord

type TxConverter func(tx *Transaction) (GenericTransaction, error)
type TxConverters map[string]TxConverter

type Module interface {
	TxExecutors() []TxExecutor
	GenericTransactionValidator() GenericTransactionValidator
	TxConverter() TxConverters
}

type GenericTxSystem struct {
	systemIdentifier   []byte
	hashAlgorithm      crypto.Hash
	trustBase          map[string]abcrypto.Verifier
	systemDescriptions SystemDescriptions
	state              *rma.Tree
	currentBlockNumber uint64

	executors           TxExecutors
	txConverters        TxConverters
	genericTxValidators []GenericTransactionValidator
	beginBlockFunctions []func(blockNumber uint64)
	endBlockFunctions   []func(blockNumber uint64) error
}

func NewModularTxSystem(modules []Module, opts ...Option) (*GenericTxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	txs := &GenericTxSystem{
		systemIdentifier:    options.systemIdentifier,
		hashAlgorithm:       options.hashAlgorithm,
		trustBase:           options.trustBase,
		systemDescriptions:  options.systemDescriptions,
		state:               options.state,
		beginBlockFunctions: options.beginBlockFunctions,
		endBlockFunctions:   options.endBlockFunctions,
		executors:           map[reflect.Type]ExecuteFunc{},
		txConverters:        make(map[string]TxConverter),
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
		for _, executor := range executors {
			txs.executors[executor.Type()] = executor.ExecuteFunc()
		}
		converters := module.TxConverter()
		for key, converter := range converters {
			txs.txConverters[key] = converter
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

func (m *GenericTxSystem) State() (State, error) {
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

func (m *GenericTxSystem) ConvertTx(tx *Transaction) (GenericTransaction, error) {
	if tx == nil || tx.TransactionAttributes == nil {
		return nil, errors.New("tx or tx attributes missing")
	}
	typeUrl := tx.TransactionAttributes.TypeUrl
	c, f := m.txConverters[typeUrl]
	if !f {
		return nil, fmt.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
	transaction, err := c(tx)
	if err != nil {
		fmt.Errorf("failed to convert tx with attributres type url '%s': %w", tx.TransactionAttributes.TypeUrl, err)
	}
	return transaction, nil
}

func (m *GenericTxSystem) Execute(tx GenericTransaction) error {
	u, _ := m.state.GetUnit(tx.UnitID())
	ctx := &TxValidationContext{
		Tx:               tx,
		Bd:               u,
		SystemIdentifier: m.systemIdentifier,
		BlockNumber:      m.currentBlockNumber,

		FeeCreditRecord: m.getFCR(tx.ToProtoBuf().ClientMetadata),
		TxFee:           1, // TODO revisit: add fee function
	}
	for _, validator := range m.genericTxValidators {
		if err := validator(ctx); err != nil {
			return fmt.Errorf("invalid transaction: %w", err)
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

// TODO move
func (m *GenericTxSystem) getFCR(clientMD *ClientMetadata) *rma.Unit {
	var fcr *rma.Unit
	if len(clientMD.FeeCreditRecordId) > 0 {
		fcr, _ = m.state.GetUnit(uint256.NewInt(0).SetBytes(clientMD.FeeCreditRecordId))
	}
	return fcr
}
