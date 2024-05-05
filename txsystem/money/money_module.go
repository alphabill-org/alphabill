package money

import (
	"crypto"
	"errors"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

var _ txsystem.Module = (*Module)(nil)

type (
	Module struct {
		state               *state.State
		systemID            types.SystemID
		trustBase           types.RootTrustBase
		hashAlgorithm       crypto.Hash
		dustCollector       *DustCollector
		feeCreditTxRecorder *feeCreditTxRecorder
		feeCalculator       fc.FeeCalculator
		execPredicate       func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error
	}
)

func NewMoneyModule(options *Options) (*Module, error) {
	if options == nil {
		return nil, errors.New("money module options are missing")
	}
	if options.state == nil {
		return nil, errors.New("state is nil")
	}
	if options.feeCalculator == nil {
		return nil, errors.New("fee calculator function is nil")
	}

	m := &Module{
		state:               options.state,
		systemID:            options.systemIdentifier,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(options.state, options.systemIdentifier, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(options.state),
		feeCalculator:       options.feeCalculator,
		execPredicate:       predicates.NewPredicateRunner(options.exec, options.state),
	}
	return m, nil
}

func (m *Module) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		// money partition tx handlers
		money.PayloadTypeTransfer: m.handleTransferTx().ExecuteFunc(),
		money.PayloadTypeSplit:    m.handleSplitTx().ExecuteFunc(),
		money.PayloadTypeTransDC:  m.handleTransferDCTx().ExecuteFunc(),
		money.PayloadTypeSwapDC:   m.handleSwapDCTx().ExecuteFunc(),
		money.PayloadTypeLock:     m.handleLockTx().ExecuteFunc(),
		money.PayloadTypeUnlock:   m.handleUnlockTx().ExecuteFunc(),

		// fee credit related transaction handlers (credit transfers and reclaims only!)
		fcsdk.PayloadTypeTransferFeeCredit: m.handleTransferFeeCreditTx().ExecuteFunc(),
		fcsdk.PayloadTypeReclaimFeeCredit:  m.handleReclaimFeeCreditTx().ExecuteFunc(),
	}
}

func (m *Module) BeginBlockFuncs() []func(blockNr uint64) error {
	return []func(blockNr uint64) error{
		func(blockNr uint64) error {
			m.feeCreditTxRecorder.reset()
			return nil
		},
	}
}

func (m *Module) EndBlockFuncs() []func(blockNumber uint64) error {
	return []func(blockNumber uint64) error{
		// m.dustCollector.consolidateDust TODO AB-1133
		// TODO AB-1133 delete bills from owner index (partition/proof_indexer.go)
		func(blockNr uint64) error {
			return m.feeCreditTxRecorder.consolidateFees()
		},
	}
}
