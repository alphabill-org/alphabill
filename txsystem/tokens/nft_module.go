package tokens

import (
	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
)

var _ txsystem2.Module = &NonFungibleTokensModule{}

type NonFungibleTokensModule struct {
	txExecutors map[string]txsystem2.TxExecutor
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{
		txExecutors: map[string]txsystem2.TxExecutor{
			PayloadTypeCreateNFTType: handleCreateNoneFungibleTokenTx(options),
			PayloadTypeMintNFT:       handleMintNonFungibleTokenTx(options),
			PayloadTypeTransferNFT:   handleTransferNonFungibleTokenTx(options),
			PayloadTypeUpdateNFT:     handleUpdateNonFungibleTokenTx(options),
		},
	}, nil
}

func (n *NonFungibleTokensModule) TxExecutors() map[string]txsystem2.TxExecutor {
	return n.txExecutors
}

func (n *NonFungibleTokensModule) GenericTransactionValidator() txsystem2.GenericTransactionValidator {
	return ValidateGenericTransaction
}
