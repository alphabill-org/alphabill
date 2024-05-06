package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

var _ txsystem.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	state         *state.State
	feeCalculator fc.FeeCalculator
	hashAlgorithm crypto.Hash
	trustBase     types.RootTrustBase
	execPredicate predicates.PredicateRunner
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		state:         options.state,
		feeCalculator: options.feeCalculator,
		hashAlgorithm: options.hashAlgorithm,
		trustBase:     options.trustBase,
		execPredicate: PredicateRunner(options.exec),
	}, nil
}

func (m *FungibleTokensModule) TxExecutors() map[string]txsystem.ExecuteFunc {
	return map[string]txsystem.ExecuteFunc{
		tokens.PayloadTypeCreateFungibleTokenType: m.handleCreateFungibleTokenTypeTx().ExecuteFunc(),
		tokens.PayloadTypeMintFungibleToken:       m.handleMintFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeTransferFungibleToken:   m.handleTransferFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeSplitFungibleToken:      m.handleSplitFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeBurnFungibleToken:       m.handleBurnFungibleTokenTx().ExecuteFunc(),
		tokens.PayloadTypeJoinFungibleToken:       m.handleJoinFungibleTokenTx().ExecuteFunc(),
	}
}
