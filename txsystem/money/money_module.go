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
		execPredicate       predicates.PredicateRunner
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

func (m *Module) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		// money partition tx handlers
		money.PayloadTypeTransfer: txsystem.NewTxHandler[money.TransferAttributes](m.validateTransferTx, m.executeTransferTx),
		money.PayloadTypeSplit:    txsystem.NewTxHandler[money.SplitAttributes](m.validateSplitTx, m.executeSplitTx),
		money.PayloadTypeTransDC:  txsystem.NewTxHandler[money.TransferDCAttributes](m.validateTransferDCTx, m.executeTransferDCTx),
		money.PayloadTypeSwapDC:   txsystem.NewTxHandler[money.SwapDCAttributes](m.validateSwapTx, m.executeSwapTx),
		money.PayloadTypeLock:     txsystem.NewTxHandler[money.LockAttributes](m.validateLockTx, m.executeLockTx),
		money.PayloadTypeUnlock:   txsystem.NewTxHandler[money.UnlockAttributes](m.validateUnlockTx, m.executeUnlockTx),
		// fee credit related transaction handlers (credit transfers and reclaims only!)
		fcsdk.PayloadTypeTransferFeeCredit: txsystem.NewTxHandler[fcsdk.TransferFeeCreditAttributes](m.validateTransferFCTx, m.executeTransferFCTx),
		fcsdk.PayloadTypeReclaimFeeCredit:  txsystem.NewTxHandler[fcsdk.ReclaimFeeCreditAttributes](m.validateReclaimFCTx, m.executeReclaimFCTx),
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
