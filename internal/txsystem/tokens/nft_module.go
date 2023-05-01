package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = &NonFungibleTokensModule{}

type NonFungibleTokensModule struct {
	txExecutors []txsystem.TxExecutor
	txConverter txsystem.TxConverters
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{
		txExecutors: []txsystem.TxExecutor{
			handleCreateNoneFungibleTokenTx(options),
			handleMintNonFungibleTokenTx(options),
			handleTransferNonFungibleTokenTx(options),
			handleUpdateNonFungibleTokenTx(options),
		},
		txConverter: map[string]txsystem.TxConverter{
			typeURLCreateNonFungibleTokenTypeAttributes: convertCreateNonFungibleTokenType,
			typeURLMintNonFungibleTokenAttributes:       convertMintNonFungibleToken,
			typeURLTransferNonFungibleTokenAttributes:   convertTransferNonFungibleToken,
			typeURLUpdateNonFungibleTokenAttributes:     convertUpdateNonFungibleToken,
		},
	}, nil
}

func (n *NonFungibleTokensModule) TxExecutors() []txsystem.TxExecutor {
	return n.txExecutors
}

func (n *NonFungibleTokensModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return ValidateGenericTransaction
}

func (n *NonFungibleTokensModule) TxConverter() txsystem.TxConverters {
	return n.txConverter
}
