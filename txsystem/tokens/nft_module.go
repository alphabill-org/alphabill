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
		execPredicate: predicates.NewPredicateRunner(options.exec),
	}, nil
}

func (n *NonFungibleTokensModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		tokens.TransactionTypeDefineNFT:   txtypes.NewTxHandler[tokens.DefineNonFungibleTokenAttributes, tokens.DefineNonFungibleTokenAuthProof](n.validateDefineNFT, n.executeDefineNFT),
		tokens.TransactionTypeMintNFT:     txtypes.NewTxHandler[tokens.MintNonFungibleTokenAttributes, tokens.MintNonFungibleTokenAuthProof](n.validateMintNFT, n.executeMintNFT),
		tokens.TransactionTypeTransferNFT: txtypes.NewTxHandler[tokens.TransferNonFungibleTokenAttributes, tokens.TransferNonFungibleTokenAuthProof](n.validateTransferNFT, n.executeTransferNFT),
		tokens.TransactionTypeUpdateNFT:   txtypes.NewTxHandler[tokens.UpdateNonFungibleTokenAttributes, tokens.UpdateNonFungibleTokenAuthProof](n.validateUpdateNFT, n.executeUpdateNFT),
	}
}
