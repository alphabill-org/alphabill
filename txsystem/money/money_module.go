package money

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/api/types"
	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/state"
)

var _ txsystem2.Module = (*Module)(nil)

var (
	ErrInitialBillIsNil                  = errors.New("initial bill may not be nil")
	ErrInvalidInitialBillID              = errors.New("initial bill ID may not be equal to the DC money supply ID")
	ErrUndefinedSystemDescriptionRecords = errors.New("undefined system description records")
	ErrNilFeeCreditBill                  = errors.New("fee credit bill is nil in system description record")
	ErrInvalidFeeCreditBillID            = errors.New("fee credit bill may not be equal to the DC money supply ID and initial bill ID")
)

type (
	Module struct {
		state               *state.State
		systemID            []byte
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
	s := options.state

	if options.feeCalculator == nil {
		return nil, errors.New("fee calculator function is nil")
	}
	savepointID := s.Savepoint()
	defer func() {
		if err != nil {
			s.RollbackToSavepoint(savepointID)
			return
		}
		s.ReleaseToSavepoint(savepointID)
		_, _, err = s.CalculateRoot()
		if err != nil {
			return
		}
		err = s.Commit()
	}()
	if err = addInitialBill(options.initialBill, s); err != nil {
		return nil, fmt.Errorf("could not set initial bill: %w", err)
	}

	if err = addInitialDustCollectorMoneySupply(options.dcMoneyAmount, s); err != nil {
		return nil, fmt.Errorf("could not set DC money supply: %w", err)
	}

	if err = addInitialFeeCredits(options.systemDescriptionRecords, options.initialBill.ID, s); err != nil {
		return nil, fmt.Errorf("could not set initial fee credits: %w", err)
	}
	m = &Module{
		state:               s,
		systemID:            options.systemIdentifier,
		trustBase:           options.trustBase,
		hashAlgorithm:       options.hashAlgorithm,
		feeCreditTxRecorder: newFeeCreditTxRecorder(s, options.systemIdentifier, options.systemDescriptionRecords),
		dustCollector:       NewDustCollector(s),
		feeCalculator:       options.feeCalculator,
	}
	return
}

func (m *Module) TxExecutors() map[string]txsystem2.TxExecutor {
	return map[string]txsystem2.TxExecutor{
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

func (m *Module) GenericTransactionValidator() txsystem2.GenericTransactionValidator {
	return txsystem2.ValidateGenericTransaction
}

func addInitialBill(initialBill *InitialBill, s *state.State) error {
	if initialBill == nil {
		return ErrInitialBillIsNil
	}
	if dustCollectorMoneySupplyID.Eq(initialBill.ID) {
		return ErrInvalidInitialBillID
	}
	return s.Apply(state.AddUnit(initialBill.ID, initialBill.Owner, &BillData{
		V:        initialBill.Value,
		T:        0,
		Backlink: nil,
	}))
}

func addInitialFeeCredits(records []*genesis.SystemDescriptionRecord, initialBillID types.UnitID, s *state.State) error {
	if len(records) == 0 {
		return ErrUndefinedSystemDescriptionRecords
	}
	for _, sdr := range records {
		feeCreditBill := sdr.FeeCreditBill
		if feeCreditBill == nil {
			return ErrNilFeeCreditBill
		}
		if bytes.Equal(feeCreditBill.UnitId, dustCollectorMoneySupplyID) || bytes.Equal(feeCreditBill.UnitId, initialBillID) {
			return ErrInvalidFeeCreditBillID
		}
		err := s.Apply(state.AddUnit(feeCreditBill.UnitId, feeCreditBill.OwnerPredicate, &BillData{
			V:        0,
			T:        0,
			Backlink: nil,
		}))
		if err != nil {
			return err
		}
	}
	return nil
}

func addInitialDustCollectorMoneySupply(dcMoneyAmount uint64, s *state.State) error {
	return s.Apply(state.AddUnit(dustCollectorMoneySupplyID, dustCollectorPredicate, &BillData{
		V:        dcMoneyAmount,
		T:        0,
		Backlink: nil,
	}))
}
