package money

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

var (
	log = logger.CreateForPackage()

	ErrInitialBillIsNil                  = errors.New("initial bill may not be nil")
	ErrInvalidInitialBillID              = errors.New("initial bill ID may not be equal to the DC money supply ID")
	ErrUndefinedSystemDescriptionRecords = errors.New("undefined system description records")
	ErrNilFeeCreditBill                  = errors.New("fee credit bill is nil in system description record")
	ErrInvalidFeeCreditBillID            = errors.New("fee credit bill may not be equal to the DC money supply ID and initial bill ID")
)

func NewMoneyTxSystem(systemIdentifier []byte, opts ...Option) (*txsystem.ModularTxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	money, err := NewMoneyModule(systemIdentifier, options)
	if err != nil {
		// TODO test
		return nil, fmt.Errorf("failed to load money module: %w", err)
	}

	feeCredit, err := fc.NewFeeCreditModule(
		fc.WithState(options.state),
		fc.WithHashAlgorithm(options.hashAlgorithm),
		fc.WithTrustBase(options.trustBase),
		fc.WithSystemIdentifier(systemIdentifier),
		fc.WithMoneyTXSystemIdentifier(systemIdentifier),
		fc.WithFeeCalculator(options.feeCalculator),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}

	return txsystem.NewModularTxSystem(
		[]txsystem.Module{money, feeCredit},
		txsystem.WithEndBlockFunctions(money.EndBlockFuncs()),
		txsystem.WithBeginBlockFunctions(money.BeginBlockFuncs()),
		txsystem.WithSystemIdentifier(systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithTrustBase(options.trustBase),
		txsystem.WithState(options.state),
	)
}
