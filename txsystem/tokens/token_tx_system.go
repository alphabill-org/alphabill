package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	basetypes "github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/permissioned"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
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
	ErrStrStateIsNil            = "state is nil"
	ErrStrInvalidSymbolLength   = "symbol length exceeds the allowed maximum of 16 bytes"
	ErrStrInvalidNameLength     = "name length exceeds the allowed maximum of 256 bytes"
	ErrStrInvalidIconTypeLength = "icon type length exceeds the allowed maximum of 64 bytes"
	ErrStrInvalidIconDataLength = "icon data length exceeds the allowed maximum of 64 KiB"
)

func NewTxSystem(pdr basetypes.PartitionDescriptionRecord, shardID basetypes.ShardID, observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("tokens transaction system default config: %w", err)
	}

	for _, opt := range opts {
		opt(options)
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

	var feeCreditModule txtypes.FeeCreditModule
	if len(options.adminOwnerPredicate) > 0 {
		feeCreditModule, err = permissioned.NewFeeCreditModule(
			pdr.SystemIdentifier, options.state, tokens.FeeCreditRecordUnitType, options.adminOwnerPredicate,
			permissioned.WithHashAlgorithm(options.hashAlgorithm),
			permissioned.WithFeelessMode(options.feelessMode),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissioned fee credit module: %w", err)
		}
	} else {
		feeCreditModule, err = fc.NewFeeCreditModule(
			fc.WithState(options.state),
			fc.WithHashAlgorithm(options.hashAlgorithm),
			fc.WithTrustBase(options.trustBase),
			fc.WithSystemID(pdr.SystemIdentifier),
			fc.WithMoneySystemID(options.moneySystemID),
			fc.WithFeeCreditRecordUnitType(tokens.FeeCreditRecordUnitType),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissionless fee credit module: %w", err)
		}
	}
	return txsystem.NewGenericTxSystem(
		pdr,
		shardID,
		options.trustBase,
		[]txtypes.Module{nft, fungible, lockTokens},
		observe,
		txsystem.WithFeeCredits(feeCreditModule),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
