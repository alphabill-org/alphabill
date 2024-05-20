package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

const (
	zeroSummaryValue  uint64 = 0
	uriMaxSize               = 4 * 1024
	dataMaxSize              = 64 * 1024
	maxSymbolLength          = 16
	maxNameLength            = 256
	maxIconTypeLength        = 64
	maxIconDataLength        = 64 * 1024
	maxDecimalPlaces         = 8

	ErrStrInvalidUnitID         = "invalid unit ID"
	ErrStrInvalidTokenTypeID    = "invalid token type ID"
	ErrStrInvalidParentTypeID   = "invalid parent type ID"
	ErrStrInvalidSystemID       = "system identifier is not assigned"
	ErrStrStateIsNil            = "state is nil"
	ErrStrInvalidSymbolLength   = "symbol length exceeds the allowed maximum of 16 bytes"
	ErrStrInvalidNameLength     = "name length exceeds the allowed maximum of 256 bytes"
	ErrStrInvalidIconTypeLength = "icon type length exceeds the allowed maximum of 64 bytes"
	ErrStrInvalidIconDataLength = "icon data length exceeds the allowed maximum of 64 KiB"
)

func NewTxSystem(observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("tokens tx system default config: %w", err)
	}

	for _, opt := range opts {
		opt(options)
	}
	if options.systemIdentifier == 0 {
		return nil, errors.New(ErrStrInvalidSystemID)
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
	lockTokens, err := NewLockTokensModule(options)
	if err != nil {
		return nil, fmt.Errorf("failed to load lock tokens module: %w", err)
	}
	feeCreditModule, err := fc.NewFeeCreditModule(
		fc.WithState(options.state),
		fc.WithHashAlgorithm(options.hashAlgorithm),
		fc.WithTrustBase(options.trustBase),
		fc.WithSystemIdentifier(options.systemIdentifier),
		fc.WithMoneySystemIdentifier(options.moneyTXSystemIdentifier),
		fc.WithFeeCalculator(options.feeCalculator),
		fc.WithFeeCreditRecordUnitType(tokens.FeeCreditRecordUnitType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		options.systemIdentifier,
		feeCreditModule.CheckFeeCreditBalance,
		[]txsystem.Module{nft, fungible, lockTokens, feeCreditModule},
		observe,
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
