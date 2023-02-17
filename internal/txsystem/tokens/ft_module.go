package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	txExecutors []txsystem.TxExecutor
	txConverter txsystem.TxConverters
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		txExecutors: []txsystem.TxExecutor{
			// Fungible Token transaction executors
			handleCreateFungibleTokenTypeTx(options),
			handleMintFungibleTokenTx(options),
			handleTransferFungibleTokenTx(options),
			handleSplitFungibleTokenTx(options),
			handleBurnFungibleTokenTx(options),
			handleJoinFungibleTokenTx(options),
		},
		txConverter: map[string]txsystem.TxConverter{
			typeURLCreateFungibleTokenTypeAttributes: convertCreateFungibleTokenType,
			typeURLMintFungibleTokenAttributes:       convertMintFungibleToken,
			typeURLTransferFungibleTokenAttributes:   convertTransferFungibleToken,
			typeURLSplitFungibleTokenAttributes:      convertSplitFungibleToken,
			typeURLBurnFungibleTokenAttributes:       convertBurnFungibleToken,
			typeURLJoinFungibleTokenAttributes:       convertJoinFungibleToken,
		},
	}, nil
}

func (n *FungibleTokensModule) TxExecutors() []txsystem.TxExecutor {
	return n.txExecutors
}

func (n *FungibleTokensModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return ValidateGenericTransaction
}

func (n *FungibleTokensModule) TxConverter() txsystem.TxConverters {
	return n.txConverter
}
