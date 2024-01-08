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

func (m *Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		// money partition tx handlers
		PayloadTypeTransfer: handleTransferTx(m.state, m.hashAlgorithm, m.feeCalculator),
		PayloadTypeSplit:    handleSplitTx(m.state, m.hashAlgorithm, m.feeCalculator),
		PayloadTypeTransDC:  handleTransferDCTx(m.state, m.dustCollector, m.hashAlgorithm, m.feeCalculator),
		PayloadTypeSwapDC:   handleSwapDCTx(m.state, m.systemID, m.hashAlgorithm, m.trustBase, m.feeCalculator),
		PayloadTypeLock:     handleLockTx(m.state, m.hashAlgorithm, m.feeCalculator),
		PayloadTypeUnlock:   handleUnlockTx(m.state, m.hashAlgorithm, m.feeCalculator),

		// fee credit related transaction handlers (credit transfers and reclaims only!)
		transactions.PayloadTypeTransferFeeCredit: handleTransferFeeCreditTx(m.state, m.hashAlgorithm, m.feeCreditTxRecorder, m.feeCalculator),
		transactions.PayloadTypeReclaimFeeCredit:  handleReclaimFeeCreditTx(m.state, m.hashAlgorithm, m.trustBase, m.feeCreditTxRecorder, m.feeCalculator),
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
		func(blockNr uint64) error {
			return m.feeCreditTxRecorder.consolidateFees()
		},
	}
}

func (m *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
}
