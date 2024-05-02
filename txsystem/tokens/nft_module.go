package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

var _ txsystem.Module = (*NonFungibleTokensModule)(nil)

type NonFungibleTokensModule struct {
	state         *state.State
	feeCalculator fc.FeeCalculator
	hashAlgorithm crypto.Hash
	execPredicate predicates.PredicateRunner
}

func NewNonFungibleTokensModule(options *Options) (*NonFungibleTokensModule, error) {
	return &NonFungibleTokensModule{
		state:         options.state,
		feeCalculator: options.feeCalculator,
		hashAlgorithm: options.hashAlgorithm,
		execPredicate: PredicateRunner(options.exec, options.state),
	}, nil
}

func (n *NonFungibleTokensModule) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		tokens.PayloadTypeCreateNFTType: txsystem.NewTxHandler[tokens.CreateNonFungibleTokenTypeAttributes](n.validateCreateNFTType, n.executeCreateNFTType),
		tokens.PayloadTypeMintNFT:       txsystem.NewTxHandler[tokens.MintNonFungibleTokenAttributes](n.validateNFTMintTx, n.executeNFTMintTx),
		tokens.PayloadTypeTransferNFT:   txsystem.NewTxHandler[tokens.TransferNonFungibleTokenAttributes](n.validateNFTTransferTx, n.executeNFTTransferTx),
		tokens.PayloadTypeUpdateNFT:     txsystem.NewTxHandler[tokens.UpdateNonFungibleTokenAttributes](n.validateNFTUpdateTx, n.executeNFTUpdateTx),
	}
}
