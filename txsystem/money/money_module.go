package money

import (
	"crypto"
	"errors"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/types"
)

var _ txsystem.Module = (*Module)(nil)

type (
	Module struct {
		state               *state.State
		systemID            types.SystemID
		trustBase           map[string]abcrypto.Verifier
		hashAlgorithm       crypto.Hash
		dustCollector       *DustCollector
		feeCreditTxRecorder *feeCreditTxRecorder
		feeCalculator       fc.FeeCalculator
	}
)

func NewMoneyModule(options *Options) (m *Module, err error) {
	if options == nil {
		return nil, errors.New("money module options are missing")
	}
	if options.state == nil {
		return nil, errors.New("state is nil")
	}
	if options.feeCalculator == nil {
		return nil, errors.New("fee calculator function is nil")
	}
	m = &Module{
		state:               options.state,
		systemID:            options.systemIdentifier,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(options.state, options.systemIdentifier, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(options.state),
		feeCalculator:       options.feeCalculator,
	}
	return
}

func (m *Module) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		// money partition tx handlers
		PayloadTypeTransfer: handleTransferTx(m.state, m.hashAlgorithm, m.feeCalculator).ExecuteFunc(),
		PayloadTypeSplit:    handleSplitTx(m.state, m.hashAlgorithm, m.feeCalculator).ExecuteFunc(),
		PayloadTypeTransDC:  handleTransferDCTx(m.state, m.dustCollector, m.hashAlgorithm, m.feeCalculator).ExecuteFunc(),
		PayloadTypeSwapDC:   handleSwapDCTx(m.state, m.systemID, m.hashAlgorithm, m.trustBase, m.feeCalculator).ExecuteFunc(),
		PayloadTypeLock:     handleLockTx(m.state, m.hashAlgorithm, m.feeCalculator).ExecuteFunc(),
		PayloadTypeUnlock:   handleUnlockTx(m.state, m.hashAlgorithm, m.feeCalculator).ExecuteFunc(),

		// fee credit related transaction handlers (credit transfers and reclaims only!)
		transactions.PayloadTypeTransferFeeCredit: handleTransferFeeCreditTx(m.state, m.hashAlgorithm, m.feeCreditTxRecorder, m.feeCalculator).ExecuteFunc(),
		transactions.PayloadTypeReclaimFeeCredit:  handleReclaimFeeCreditTx(m.state, m.hashAlgorithm, m.trustBase, m.feeCreditTxRecorder, m.feeCalculator).ExecuteFunc(),
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
