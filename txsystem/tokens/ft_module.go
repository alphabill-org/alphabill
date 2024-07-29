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
		execPredicate: PredicateRunner(options.exec),
	}, nil
}

func (m *FungibleTokensModule) TxHandlers() map[string]txtypes.TxExecutor {
	return map[string]txtypes.TxExecutor{
		tokens.PayloadTypeCreateFungibleTokenType: txtypes.NewTxHandler[tokens.CreateFungibleTokenTypeAttributes](m.validateCreateFTType, m.executeCreateFTType),
		tokens.PayloadTypeMintFungibleToken:       txtypes.NewTxHandler[tokens.MintFungibleTokenAttributes](m.validateMintFT, m.executeMintFT),
		tokens.PayloadTypeTransferFungibleToken:   txtypes.NewTxHandler[tokens.TransferFungibleTokenAttributes](m.validateTransferFT, m.executeTransferFT),
		tokens.PayloadTypeSplitFungibleToken:      txtypes.NewTxHandler[tokens.SplitFungibleTokenAttributes](m.validateSplitFT, m.executeSplitFT),
		tokens.PayloadTypeBurnFungibleToken:       txtypes.NewTxHandler[tokens.BurnFungibleTokenAttributes](m.validateBurnFT, m.executeBurnFT),
		tokens.PayloadTypeJoinFungibleToken:       txtypes.NewTxHandler[tokens.JoinFungibleTokenAttributes](m.validateJoinFT, m.executeJoinFT),
	}
}
