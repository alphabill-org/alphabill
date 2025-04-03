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
	pdr           types.PartitionDescriptionRecord
}

func NewFungibleTokensModule(pdr types.PartitionDescriptionRecord, options *Options) (*FungibleTokensModule, error) {
	return &FungibleTokensModule{
		state:         options.state,
		hashAlgorithm: options.hashAlgorithm,
		trustBase:     options.trustBase,
		execPredicate: predicates.NewPredicateRunner(options.exec),
		pdr:           pdr,
	}, nil
}

func (m *FungibleTokensModule) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		tokens.TransactionTypeDefineFT:   txtypes.NewTxHandler[tokens.DefineFungibleTokenAttributes, tokens.DefineFungibleTokenAuthProof](m.validateDefineFT, m.executeDefineFT),
		tokens.TransactionTypeMintFT:     txtypes.NewTxHandler[tokens.MintFungibleTokenAttributes, tokens.MintFungibleTokenAuthProof](m.validateMintFT, m.executeMintFT),
		tokens.TransactionTypeTransferFT: txtypes.NewTxHandler[tokens.TransferFungibleTokenAttributes, tokens.TransferFungibleTokenAuthProof](m.validateTransferFT, m.executeTransferFT),
		tokens.TransactionTypeSplitFT:    txtypes.NewTxHandler[tokens.SplitFungibleTokenAttributes, tokens.SplitFungibleTokenAuthProof](m.validateSplitFT, m.executeSplitFT, txtypes.WithTargetUnitsFn(m.splitFTTargetUnits)),
		tokens.TransactionTypeBurnFT:     txtypes.NewTxHandler[tokens.BurnFungibleTokenAttributes, tokens.BurnFungibleTokenAuthProof](m.validateBurnFT, m.executeBurnFT),
		tokens.TransactionTypeJoinFT:     txtypes.NewTxHandler[tokens.JoinFungibleTokenAttributes, tokens.JoinFungibleTokenAuthProof](m.validateJoinFT, m.executeJoinFT),
	}
}
