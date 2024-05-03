package tokens

import (
	"crypto"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
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
		execPredicate: PredicateRunner(options.exec, options.state),
	}, nil
}

func (m *FungibleTokensModule) TxHandlers() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		tokens.PayloadTypeCreateFungibleTokenType: txsystem.NewTxHandler[tokens.CreateFungibleTokenTypeAttributes](m.validateCreateFTType, m.executeCreateFTType),
		tokens.PayloadTypeMintFungibleToken:       txsystem.NewTxHandler[tokens.MintFungibleTokenAttributes](m.validateMintFT, m.executeMintFT),
		tokens.PayloadTypeTransferFungibleToken:   txsystem.NewTxHandler[tokens.TransferFungibleTokenAttributes](m.validateTransferFT, m.executeTransferFT),
		tokens.PayloadTypeSplitFungibleToken:      txsystem.NewTxHandler[tokens.SplitFungibleTokenAttributes](m.validateSplitFT, m.executeSplitFT),
		tokens.PayloadTypeBurnFungibleToken:       txsystem.NewTxHandler[tokens.BurnFungibleTokenAttributes](m.validateBurnFT, m.executeBurnFT),
		tokens.PayloadTypeJoinFungibleToken:       txsystem.NewTxHandler[tokens.JoinFungibleTokenAttributes](m.validateJoinFT, m.executeJoinFT),
	}
}
