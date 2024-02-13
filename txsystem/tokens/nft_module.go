package tokens

import "github.com/alphabill-org/alphabill/txsystem"

var _ txsystem.Module = (*NonFungibleTokensModule)(nil)

type NonFungibleTokensModule struct {
	options *Options
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{options: options}, nil
}

func (n *NonFungibleTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeCreateNFTType: handleCreateNoneFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeMintNFT:       handleMintNonFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeTransferNFT:   handleTransferNonFungibleTokenTx(n.options).ExecuteFunc(),
		PayloadTypeUpdateNFT:     handleUpdateNonFungibleTokenTx(n.options).ExecuteFunc(),
	}
}
