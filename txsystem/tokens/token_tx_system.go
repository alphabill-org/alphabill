package tokens

import (
	"errors"
	"fmt"
	"log/slog"

	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
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
	ErrStrInvalidTypeID         = "invalid type ID"
	ErrStrInvalidParentTypeID   = "invalid parent type ID"
	ErrStrSystemIdentifierIsNil = "system identifier is nil"
	ErrStrStateIsNil            = "state is nil"
	ErrStrInvalidSymbolLength   = "symbol length exceeds the allowed maximum of 16 bytes"
	ErrStrInvalidNameLength     = "name length exceeds the allowed maximum of 256 bytes"
	ErrStrInvalidIconTypeLength = "icon type length exceeds the allowed maximum of 64 bytes"
	ErrStrInvalidIconDataLength = "icon data length exceeds the allowed maximum of 64 KiB"
)

func NewTxSystem(log *slog.Logger, opts ...Option) (*txsystem2.GenericTxSystem, error) {
	options := defaultOptions()
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
		fc.WithFeeCreditRecordUnitType(FeeCreditRecordUnitType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load fee credit module: %w", err)
	}
	return txsystem2.NewGenericTxSystem(
		log,
		[]txsystem2.Module{nft, fungible, lockTokens, feeCreditModule},
		txsystem2.WithSystemIdentifier(options.systemIdentifier),
		txsystem2.WithHashAlgorithm(options.hashAlgorithm),
		txsystem2.WithState(options.state),
	)
}