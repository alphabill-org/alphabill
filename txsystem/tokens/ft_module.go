package tokens

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
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
	trustBase     map[string]abcrypto.Verifier
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
		PayloadTypeCreateFungibleTokenType: m.handleCreateFungibleTokenTypeTx().ExecuteFunc(),
		PayloadTypeMintFungibleToken:       m.handleMintFungibleTokenTx().ExecuteFunc(),
		PayloadTypeTransferFungibleToken:   m.handleTransferFungibleTokenTx().ExecuteFunc(),
		PayloadTypeSplitFungibleToken:      m.handleSplitFungibleTokenTx().ExecuteFunc(),
		PayloadTypeBurnFungibleToken:       m.handleBurnFungibleTokenTx().ExecuteFunc(),
		PayloadTypeJoinFungibleToken:       m.handleJoinFungibleTokenTx().ExecuteFunc(),
	}
}
