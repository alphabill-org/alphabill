package tokens

import (
	"github.com/alphabill-org/alphabill/validator/internal/txsystem"
)

var _ txsystem.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	txExecutors map[string]txsystem.TxExecutor
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		txExecutors: map[string]txsystem.TxExecutor{
			PayloadTypeCreateFungibleTokenType: handleCreateFungibleTokenTypeTx(options),
			PayloadTypeMintFungibleToken:       handleMintFungibleTokenTx(options),
			PayloadTypeTransferFungibleToken:   handleTransferFungibleTokenTx(options),
			PayloadTypeSplitFungibleToken:      handleSplitFungibleTokenTx(options),
			PayloadTypeBurnFungibleToken:       handleBurnFungibleTokenTx(options),
			PayloadTypeJoinFungibleToken:       handleJoinFungibleTokenTx(options),
		},
	}, nil
}

func (n *FungibleTokensModule) TxExecutors() map[string]txsystem.TxExecutor {
	return n.txExecutors
}

func (n *FungibleTokensModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return ValidateGenericTransaction
}
