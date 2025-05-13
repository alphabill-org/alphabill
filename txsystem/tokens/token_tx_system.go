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
)

var (
	errInvalidSymbolLength   = fmt.Errorf("symbol length exceeds the allowed maximum of %d bytes", maxSymbolLength)
	errInvalidNameLength     = fmt.Errorf("name length exceeds the allowed maximum of %d bytes", maxNameLength)
	errInvalidIconTypeLength = fmt.Errorf("icon type length exceeds the allowed maximum of %d bytes", maxIconTypeLength)
	errInvalidIconDataLength = fmt.Errorf("icon data length exceeds the allowed maximum of %d KiB", maxIconDataLength/1024)
)

func NewTxSystem(shardConf basetypes.PartitionDescriptionRecord, observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions(observe)
	if err != nil {
		return nil, fmt.Errorf("tokens transaction system default config: %w", err)
	}

	for _, opt := range opts {
		opt(options)
	}
	if options.state == nil {
		return nil, errors.New("state is nil")
	}

	nft, err := NewNonFungibleTokensModule(shardConf, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load NFT module: %w", err)
	}
	fungible, err := NewFungibleTokensModule(shardConf, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load fungible tokens module: %w", err)
	}

	nopModule := NewNopModule(shardConf, options)

	var feeCreditModule txtypes.FeeCreditModule
	if len(options.adminOwnerPredicate) > 0 {
		feeCreditModule, err = permissioned.NewFeeCreditModule(
			shardConf, options.state, tokens.FeeCreditRecordUnitType, options.adminOwnerPredicate, observe,
			permissioned.WithHashAlgorithm(options.hashAlgorithm),
			permissioned.WithFeelessMode(options.feelessMode),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissioned fee credit module: %w", err)
		}
	} else {
		feeCreditModule, err = fc.NewFeeCreditModule(shardConf, options.moneyPartitionID, options.state, options.trustBase, observe,
			fc.WithHashAlgorithm(options.hashAlgorithm),
			fc.WithFeeCreditRecordUnitType(tokens.FeeCreditRecordUnitType),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load permissionless fee credit module: %w", err)
		}
	}
	return txsystem.NewGenericTxSystem(
		shardConf,
		[]txtypes.Module{nft, fungible, nopModule},
		observe,
		txsystem.WithFeeCredits(feeCreditModule),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
		txsystem.WithExecutedTransactions(options.executedTransactions),
	)
}
