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
	ErrStrUnitIDIsZero          = "unit ID cannot be zero"
	ErrStrInvalidSymbolName     = "symbol name exceeds the allowed maximum length of 64 bytes"
)

func New(opts ...Option) (*txsystem.ModularTxSystem, error) {
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
	fee, err := fc.NewFeeCreditModule(
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
	modules := []txsystem.Module{nft, fungible, fee}
	return txsystem.NewModularTxSystem(
		modules,
		txsystem.WithSystemIdentifier(options.systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithTrustBase(options.trustBase),
		txsystem.WithState(options.state),
	)
}
