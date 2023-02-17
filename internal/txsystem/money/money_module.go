package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

var _ txsystem.Module = &Module{}

type (
	Module struct {
		state               *rma.Tree
		trustBase           map[string]abcrypto.Verifier
		hashAlgorithm       crypto.Hash
		dustCollector       *DustCollector
		feeCreditTxRecorder *feeCreditTxRecorder
		feeCalculator       fc.FeeCalculator
	}
)

func NewMoneyModule(systemIdentifier []byte, options *Options) (*Module, error) {
	if options == nil {
		return nil, errors.New("money module options are missing")
	}
	state := options.state

	if options.feeCalculator == nil {
		return nil, errors.New("fee calculator function is nil")
	}

	if err := addInitialBill(options.initialBill, state); err != nil {
		return nil, fmt.Errorf("could not set initial bill: %w", err)
	}

	if err := addInitialDustCollectorMoneySupply(options.dcMoneyAmount, state); err != nil {
		return nil, fmt.Errorf("could not set DC money supply: %w", err)
	}

	if err := addInitialFeeCredits(options.systemDescriptionRecords, options.initialBill.ID, state); err != nil {
		return nil, fmt.Errorf("could not set initial fee credits: %w", err)
	}
	state.Commit()
	return &Module{
		state:               state,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(state, systemIdentifier, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(state),
		feeCalculator:       options.feeCalculator,
	}, nil
}

func (m *Module) TxExecutors() []txsystem.TxExecutor {
	return []txsystem.TxExecutor{
		// money partition tx handlers
		handleTransferTx(m.state, m.hashAlgorithm, m.feeCalculator),
		handleTransferDCTx(m.state, m.dustCollector, m.hashAlgorithm, m.feeCalculator),
		handleSplitTx(m.state, m.hashAlgorithm, m.feeCalculator),
		handleSwapDCTx(m.state, m.hashAlgorithm, m.trustBase, m.feeCalculator),

		// fee credit related transaction handlers (credit transfers and reclaims only!)
		handleTransferFeeCreditTx(m.state, m.hashAlgorithm, m.feeCreditTxRecorder, m.feeCalculator),
		handleReclaimFeeCreditTx(m.state, m.hashAlgorithm, m.trustBase, m.feeCreditTxRecorder, m.feeCalculator),
	}
}

func (m *Module) BeginBlockFuncs() []func(blockNr uint64) {
	return []func(blockNr uint64){
		func(blockNr uint64) {
			m.feeCreditTxRecorder.reset()
		},
	}
}

func (m *Module) EndBlockFuncs() []func(blockNumber uint64) error {
	return []func(blockNumber uint64) error{
		m.dustCollector.consolidateDust,
		func(blockNr uint64) error {
			return m.feeCreditTxRecorder.consolidateFees()
		},
	}
}

func (m *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
}

func (m *Module) TxConverter() txsystem.TxConverters {
	return map[string]txsystem.TxConverter{
		typeURLTransferOrder:                       convertTransferTx,
		typeURLTransferDCOrder:                     convertTransferDCTx,
		typeURLSplitOrder:                          convertSplitTx,
		typeURLSwapOrder:                           convertSwapDCTx,
		transactions.TypeURLTransferFeeCreditOrder: transactions.ConvertTransferFeeCreditTx,
		transactions.TypeURLReclaimFeeCreditOrder:  transactions.ConvertReclaimFeeCreditTx,
	}
}

func addInitialBill(initialBill *InitialBill, state *rma.Tree) error {
	if initialBill == nil {
		return ErrInitialBillIsNil
	}
	if dustCollectorMoneySupplyID.Eq(initialBill.ID) {
		return ErrInvalidInitialBillID
	}
	return state.AtomicUpdate(rma.AddItem(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}, nil))
}

func addInitialFeeCredits(records []*genesis.SystemDescriptionRecord, initialBillID *uint256.Int, state *rma.Tree) error {
	if len(records) == 0 {
		return ErrUndefinedSystemDescriptionRecords
	}
	for _, sdr := range records {
		feeCreditBill := sdr.FeeCreditBill
		if feeCreditBill == nil {
			return ErrNilFeeCreditBill
		}
		if bytes.Equal(feeCreditBill.UnitId, util.Uint256ToBytes(dustCollectorMoneySupplyID)) || bytes.Equal(feeCreditBill.UnitId, util.Uint256ToBytes(initialBillID)) {
			return ErrInvalidFeeCreditBillID
		}
		return state.AtomicUpdate(rma.AddItem(uint256.NewInt(0).SetBytes(feeCreditBill.UnitId), feeCreditBill.OwnerPredicate, &BillData{
			V:        0,
			T:        0,
			Backlink: nil,
		}, nil))

	}
	return nil
}

func addInitialDustCollectorMoneySupply(dcMoneyAmount uint64, state *rma.Tree) error {
	return state.AtomicUpdate(rma.AddItem(dustCollectorMoneySupplyID, dustCollectorPredicate, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}, nil))
}
