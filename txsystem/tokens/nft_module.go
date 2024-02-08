package tokens

import "github.com/alphabill-org/alphabill/txsystem"

var _ txsystem.Module = &NonFungibleTokensModule{}

type NonFungibleTokensModule struct {
	txExecutors map[string]txsystem.TxExecutor
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{
		txExecutors: map[string]txsystem.TxExecutor{
			PayloadTypeCreateNFTType: handleCreateNoneFungibleTokenTx(options),
			PayloadTypeMintNFT:       handleMintNonFungibleTokenTx(options),
			PayloadTypeTransferNFT:   handleTransferNonFungibleTokenTx(options),
			PayloadTypeUpdateNFT:     handleUpdateNonFungibleTokenTx(options),
		},
	}, nil
}

func (n *NonFungibleTokensModule) TxExecutors() map[string]txsystem.TxExecutor {
	return n.txExecutors
}
