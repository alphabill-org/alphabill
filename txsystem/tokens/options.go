package tokens

import (
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-sdk/types"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc"
)

type (
	Options struct {
		systemIdentifier        types.SystemID
		moneyTXSystemIdentifier types.SystemID
		hashAlgorithm           gocrypto.Hash
		trustBase               map[string]crypto.Verifier
		state                   *state.State
		feeCalculator           fc.FeeCalculator
		exec                    predicates.PredicateExecutor
	}

	Option func(*Options)
)

func defaultOptions() (*Options, error) {
	predEng, err := predicates.Dispatcher(templates.New())
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	return &Options{
		systemIdentifier:        tokens.DefaultSystemID,
		moneyTXSystemIdentifier: money.DefaultSystemID,
		hashAlgorithm:           gocrypto.SHA256,
		feeCalculator:           fc.FixedFee(1),
		trustBase:               map[string]crypto.Verifier{},
		exec:                    predEng.Execute,
	}, nil
}

func WithState(s *state.State) Option {
	return func(c *Options) {
		c.state = s
	}
}

func WithSystemIdentifier(systemIdentifier types.SystemID) Option {
	return func(c *Options) {
		c.systemIdentifier = systemIdentifier
	}
}

func WithMoneyTXSystemIdentifier(moneySystemIdentifier types.SystemID) Option {
	return func(c *Options) {
		c.moneyTXSystemIdentifier = moneySystemIdentifier
	}
}

func WithFeeCalculator(calc fc.FeeCalculator) Option {
	return func(g *Options) {
		g.feeCalculator = calc
	}
}

func WithHashAlgorithm(algorithm gocrypto.Hash) Option {
	return func(c *Options) {
		c.hashAlgorithm = algorithm
	}
}

func WithTrustBase(trustBase map[string]crypto.Verifier) Option {
	return func(c *Options) {
		c.trustBase = trustBase
	}
}

/*
WithPredicateExecutor allows to replace the default predicate executor which
supports only "builtin predicate templates".
*/
func WithPredicateExecutor(exec predicates.PredicateExecutor) Option {
	return func(g *Options) {
		if exec != nil {
			g.exec = exec
		}
	}
}

/*
PredicateRunner returns token tx system specific predicate runner wrapper.
The only difference is hos the PayloadBytes method of the evaluation context
is implemented (hack is needed until AB-1012 gets resolved).
*/
func PredicateRunner(executor predicates.PredicateExecutor) predicates.PredicateRunner {
	return func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder, env predicates.TxContext) error {
		res, err := executor(context.Background(), predicate, args, txo, tokenExecEnv{env})
		if err != nil {
			return err
		}
		if !res {
			return errors.New("predicate evaluated to false")
		}
		return nil
	}
}

/*
tokenExecEnv is token tx system specific token execution environment implementation - the
main difference is the way how tx payload bytes are extracted.
*/
type tokenExecEnv struct {
	predicates.TxContext
}

/*
PayloadBytes returns txo payload bytes (bytes signed by bearer).
This hack is needed until AB-1012 gets resolved.
*/
func (tokenExecEnv) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
	attr, err := attrType(txo.PayloadType())
	if err != nil {
		return nil, err
	}
	if err := txo.Payload.UnmarshalAttributes(attr); err != nil {
		return nil, err
	}
	buf, err := txo.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return nil, err
	}
	return buf, nil
}

func attrType(payloadType string) (types.SigBytesProvider, error) {
	switch payloadType {
	case tokens.PayloadTypeCreateNFTType:
		return &tokens.CreateNonFungibleTokenTypeAttributes{}, nil
	case tokens.PayloadTypeMintNFT:
		return &tokens.MintNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeTransferNFT:
		return &tokens.TransferNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeUpdateNFT:
		return &tokens.UpdateNonFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeCreateFungibleTokenType:
		return &tokens.CreateFungibleTokenTypeAttributes{}, nil
	case tokens.PayloadTypeMintFungibleToken:
		return &tokens.MintFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeTransferFungibleToken:
		return &tokens.TransferFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeSplitFungibleToken:
		return &tokens.SplitFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeBurnFungibleToken:
		return &tokens.BurnFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeJoinFungibleToken:
		return &tokens.JoinFungibleTokenAttributes{}, nil
	case tokens.PayloadTypeLockToken:
		return &tokens.LockTokenAttributes{}, nil
	case tokens.PayloadTypeUnlockToken:
		return &tokens.UnlockTokenAttributes{}, nil
	}
	return nil, fmt.Errorf("unknown payload type %q", payloadType)
}
