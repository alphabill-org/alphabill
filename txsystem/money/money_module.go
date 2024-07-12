package money

import (
	"crypto"
	"errors"

	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
)

var _ txtypes.Module = (*Module)(nil)

type (
	Module struct {
		state               *state.State
		systemID            types.SystemID
		trustBase           types.RootTrustBase
		hashAlgorithm       crypto.Hash
		dustCollector       *DustCollector
		feeCreditTxRecorder *feeCreditTxRecorder
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

	m := &Module{
		state:               options.state,
		systemID:            options.systemIdentifier,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(options.state, options.systemIdentifier, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(options.state),
		execPredicate:       predicates.NewPredicateRunner(options.exec),
	}
	return m, nil
}

func (m *Module) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		// money partition tx handlers
		money.PayloadTypeTransfer: txtypes.NewTxHandler[money.TransferAttributes](m.validateTransferTx, m.executeTransferTx),
		money.PayloadTypeSplit:    txtypes.NewTxHandler[money.SplitAttributes](m.validateSplitTx, m.executeSplitTx),
		money.PayloadTypeTransDC:  txtypes.NewTxHandler[money.TransferDCAttributes](m.validateTransferDCTx, m.executeTransferDCTx),
		money.PayloadTypeSwapDC:   txtypes.NewTxHandler[money.SwapDCAttributes](m.validateSwapTx, m.executeSwapTx),
		money.PayloadTypeLock:     txtypes.NewTxHandler[money.LockAttributes](m.validateLockTx, m.executeLockTx),
		money.PayloadTypeUnlock:   txtypes.NewTxHandler[money.UnlockAttributes](m.validateUnlockTx, m.executeUnlockTx),
		// fee credit related transaction handlers (credit transfers and reclaims only!)
		fcsdk.PayloadTypeTransferFeeCredit: txtypes.NewTxHandler[fcsdk.TransferFeeCreditAttributes](m.validateTransferFCTx, m.executeTransferFCTx),
		fcsdk.PayloadTypeReclaimFeeCredit:  txtypes.NewTxHandler[fcsdk.ReclaimFeeCreditAttributes](m.validateReclaimFCTx, m.executeReclaimFCTx),
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
