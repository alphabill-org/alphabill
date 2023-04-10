package tokens

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)
	uriMaxSize       = 4 * 1024
	dataMaxSize      = 64 * 1024
	maxSymbolLength  = 64
	maxDecimalPlaces = 8

	ErrStrSystemIdentifierIsNil = "system identifier is nil"
	ErrStrStateIsNil            = "state is nil"
)

func New(opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(options)
	}
	if options.systemIdentifier == nil {
		return nil, errors.New(ErrStrSystemIdentifierIsNil)
	}
	if options.state == nil {
		return nil, errors.New(ErrStrStateIsNil)
	}
	nft, err := NewNonFungibleTokensModule(options)
	if err != nil {
		return nil, fmt.Errorf("failed to load NFT module: %w", err)
	}
	fungible, err := NewFungibleTokensModule(options)
	if err != nil {
		return nil, fmt.Errorf("failed to load fungible tokens module: %w", err)
	}
	modules := []txsystem.Module{nft, fungible}

	// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
	if options.feeCalculator() > 0 {
		feeCreditModule, err := fc.NewFeeCreditModule(
			fc.WithState(options.state),
			fc.WithHashAlgorithm(options.hashAlgorithm),
			fc.WithTrustBase(options.trustBase),
			fc.WithSystemIdentifier(options.systemIdentifier),
			fc.WithMoneyTXSystemIdentifier(options.systemIdentifier),
			fc.WithFeeCalculator(options.feeCalculator),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load fee credit module: %w", err)
		}
		modules = append(modules, feeCreditModule)
	}
	return txsystem.NewGenericTxSystem(
		modules,
		txsystem.WithSystemIdentifier(options.systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
