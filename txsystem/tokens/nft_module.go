package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.Module = (*NonFungibleTokensModule)(nil)

type NonFungibleTokensModule struct {
	state         *state.State
	hashAlgorithm crypto.Hash
	execPredicate predicates.PredicateRunner
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{
		state:         options.state,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: PredicateRunner(options.exec),
	}, nil
}

func (n *NonFungibleTokensModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		tokens.PayloadTypeCreateNFTType: txtypes.NewTxHandler[tokens.CreateNonFungibleTokenTypeAttributes](n.validateCreateNFTType, n.executeCreateNFTType),
		tokens.PayloadTypeMintNFT:       txtypes.NewTxHandler[tokens.MintNonFungibleTokenAttributes](n.validateNFTMintTx, n.executeNFTMintTx),
		tokens.PayloadTypeTransferNFT:   txtypes.NewTxHandler[tokens.TransferNonFungibleTokenAttributes](n.validateNFTTransferTx, n.executeNFTTransferTx),
		tokens.PayloadTypeUpdateNFT:     txtypes.NewTxHandler[tokens.UpdateNonFungibleTokenAttributes](n.validateNFTUpdateTx, n.executeNFTUpdateTx),
	}
}
