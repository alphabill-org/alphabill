package tokens

import (
	txsystem2 "github.com/alphabill-org/alphabill/txsystem"
)

var _ txsystem2.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	txExecutors map[string]txsystem2.TxExecutor
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		txExecutors: map[string]txsystem2.TxExecutor{
			PayloadTypeCreateFungibleTokenType: handleCreateFungibleTokenTypeTx(options),
			PayloadTypeMintFungibleToken:       handleMintFungibleTokenTx(options),
			PayloadTypeTransferFungibleToken:   handleTransferFungibleTokenTx(options),
			PayloadTypeSplitFungibleToken:      handleSplitFungibleTokenTx(options),
			PayloadTypeBurnFungibleToken:       handleBurnFungibleTokenTx(options),
			PayloadTypeJoinFungibleToken:       handleJoinFungibleTokenTx(options),
		},
	}, nil
}

func (n *FungibleTokensModule) TxExecutors() map[string]txsystem2.TxExecutor {
	return n.txExecutors
}

func (n *FungibleTokensModule) GenericTransactionValidator() txsystem2.GenericTransactionValidator {
	return ValidateGenericTransaction
}
