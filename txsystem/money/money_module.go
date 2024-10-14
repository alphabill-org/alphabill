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
		networkID           types.NetworkID
		systemID            types.SystemID
		trustBase           types.RootTrustBase
		hashAlgorithm       crypto.Hash
		dustCollector       *DustCollector
		feeCreditTxRecorder *feeCreditTxRecorder
		execPredicate       predicates.PredicateRunner
	}
)

func NewMoneyModule(networkID types.NetworkID, systemID types.SystemID, options *Options) (*Module, error) {
	if options == nil {
		return nil, errors.New("money module options are missing")
	}
	if options.state == nil {
		return nil, errors.New("state is nil")
	}

	m := &Module{
		state:               options.state,
		networkID:           networkID,
		systemID:            systemID,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(options.state, systemID, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(options.state),
		execPredicate:       predicates.NewPredicateRunner(options.exec),
	}
	return m, nil
}

func (m *Module) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		// money partition tx handlers
		money.TransactionTypeTransfer: txtypes.NewTxHandler[money.TransferAttributes, money.TransferAuthProof](m.validateTransferTx, m.executeTransferTx),
		money.TransactionTypeSplit:    txtypes.NewTxHandler[money.SplitAttributes, money.SplitAuthProof](m.validateSplitTx, m.executeSplitTx),
		money.TransactionTypeTransDC:  txtypes.NewTxHandler[money.TransferDCAttributes, money.TransferDCAuthProof](m.validateTransferDCTx, m.executeTransferDCTx),
		money.TransactionTypeSwapDC:   txtypes.NewTxHandler[money.SwapDCAttributes, money.SwapDCAuthProof](m.validateSwapTx, m.executeSwapTx),
		money.TransactionTypeLock:     txtypes.NewTxHandler[money.LockAttributes, money.LockAuthProof](m.validateLockTx, m.executeLockTx),
		money.TransactionTypeUnlock:   txtypes.NewTxHandler[money.UnlockAttributes, money.UnlockAuthProof](m.validateUnlockTx, m.executeUnlockTx),

		// fee credit related transaction handlers (credit transfers and reclaims only!)
		fcsdk.TransactionTypeTransferFeeCredit: txtypes.NewTxHandler[fcsdk.TransferFeeCreditAttributes, fcsdk.TransferFeeCreditAuthProof](m.validateTransferFCTx, m.executeTransferFCTx),
		fcsdk.TransactionTypeReclaimFeeCredit:  txtypes.NewTxHandler[fcsdk.ReclaimFeeCreditAttributes, fcsdk.ReclaimFeeCreditAuthProof](m.validateReclaimFCTx, m.executeReclaimFCTx),
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
