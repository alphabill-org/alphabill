package tokens

import (
	"crypto"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

var _ txtypes.Module = &FungibleTokensModule{}

type FungibleTokensModule struct {
	state         *state.State
	hashAlgorithm crypto.Hash
	trustBase     types.RootTrustBase
	execPredicate predicates.PredicateRunner
}

func NewFungibleTokensModule(options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		state:         options.state,
		hashAlgorithm: options.hashAlgorithm,
		trustBase:     options.trustBase,
		execPredicate: predicates.NewPredicateRunner(options.exec),
	}, nil
}

func (m *FungibleTokensModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		tokens.PayloadTypeDefineFT:   txtypes.NewTxHandler[tokens.DefineFungibleTokenAttributes, tokens.DefineFungibleTokenAuthProof](m.validateDefineFT, m.executeDefineFT),
		tokens.PayloadTypeMintFT:     txtypes.NewTxHandler[tokens.MintFungibleTokenAttributes, tokens.MintFungibleTokenAuthProof](m.validateMintFT, m.executeMintFT),
		tokens.PayloadTypeTransferFT: txtypes.NewTxHandler[tokens.TransferFungibleTokenAttributes, tokens.TransferFungibleTokenAuthProof](m.validateTransferFT, m.executeTransferFT),
		tokens.PayloadTypeSplitFT:    txtypes.NewTxHandler[tokens.SplitFungibleTokenAttributes, tokens.SplitFungibleTokenAuthProof](m.validateSplitFT, m.executeSplitFT),
		tokens.PayloadTypeBurnFT:     txtypes.NewTxHandler[tokens.BurnFungibleTokenAttributes, tokens.BurnFungibleTokenAuthProof](m.validateBurnFT, m.executeBurnFT),
		tokens.PayloadTypeJoinFT:     txtypes.NewTxHandler[tokens.JoinFungibleTokenAttributes, tokens.JoinFungibleTokenAuthProof](m.validateJoinFT, m.executeJoinFT),
	}
}
