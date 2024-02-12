package tokens

import "github.com/alphabill-org/alphabill/txsystem"

var _ txsystem.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	options *Options
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{options: options}, nil
}

func (n *FungibleTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeCreateFungibleTokenType: handleCreateFungibleTokenTypeTx(n.options).ExecuteFunc(),
		PayloadTypeMintFungibleToken:       handleMintFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeTransferFungibleToken:   handleTransferFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeSplitFungibleToken:      handleSplitFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeBurnFungibleToken:       handleBurnFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeJoinFungibleToken:       handleJoinFungibleTokenTx(n.options).ExecuteFunc(),
	}
}
