package tokens

import (
	"crypto"

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
		execPredicate: PredicateRunner(options.exec),
	}, nil
}

func (n *NonFungibleTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeCreateNFTType: n.handleCreateNonFungibleTokenTypeTx().ExecuteFunc(),
		PayloadTypeMintNFT:       n.handleMintNonFungibleTokenTx().ExecuteFunc(),
		PayloadTypeTransferNFT:   n.handleTransferNonFungibleTokenTx().ExecuteFunc(),
		PayloadTypeUpdateNFT:     n.handleUpdateNonFungibleTokenTx().ExecuteFunc(),
	}
}
