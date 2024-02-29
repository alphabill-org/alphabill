package tokens

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/types"
)

var _ txsystem.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	state         *state.State
	feeCalculator fc.FeeCalculator
	hashAlgorithm crypto.Hash
	trustBase     map[string]abcrypto.Verifier
	execPredicate func(predicate, args []byte, txo *types.TransactionOrder) error
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		state:         options.state,
		feeCalculator: options.feeCalculator,
		hashAlgorithm: options.hashAlgorithm,
		trustBase:     options.trustBase,
		execPredicate: PredicateRunner(options.exec, options.state),
	}, nil
}

func (n *FungibleTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		PayloadTypeCreateFungibleTokenType: n.handleCreateFungibleTokenTypeTx().ExecuteFunc(),
		PayloadTypeMintFungibleToken:       n.handleMintFungibleTokenTx().ExecuteFunc(),
		PayloadTypeTransferFungibleToken:   n.handleTransferFungibleTokenTx().ExecuteFunc(),
		PayloadTypeSplitFungibleToken:      n.handleSplitFungibleTokenTx().ExecuteFunc(),
		PayloadTypeBurnFungibleToken:       n.handleBurnFungibleTokenTx().ExecuteFunc(),
		PayloadTypeJoinFungibleToken:       n.handleJoinFungibleTokenTx().ExecuteFunc(),
	}
}
