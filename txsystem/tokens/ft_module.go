package tokens

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
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
		tokens.PayloadTypeCreateFungibleTokenType: n.handleCreateFungibleTokenTypeTx().ExecuteFunc(),
		tokens.PayloadTypeMintFungibleToken:       n.handleMintFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeTransferFungibleToken:   n.handleTransferFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeSplitFungibleToken:      n.handleSplitFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeBurnFungibleToken:       n.handleBurnFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeJoinFungibleToken:       n.handleJoinFungibleTokenTx().ExecuteFunc(),
	}
}
