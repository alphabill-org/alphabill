package tokens

import (
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
)

const DefaultSystemIdentifier types.SystemID = 0x00000002

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
		systemIdentifier:        DefaultSystemIdentifier,
		moneyTXSystemIdentifier: money.DefaultSystemIdentifier,
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
PredicateRunner returns token tx system specific predicate runner wrapper
*/
func PredicateRunner(
	executor predicates.PredicateExecutor,
	state *state.State,
) predicates.PredicateRunner {
	env := &tokenExecEnv{state: state}
	return func(predicate types.PredicateBytes, args []byte, txo *types.TransactionOrder) error {
		res, err := executor(context.Background(), predicate, args, txo, env)
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
	state *state.State
}

func (ee *tokenExecEnv) GetUnit(id types.UnitID, committed bool) (*state.Unit, error) {
	return ee.state.GetUnit(id, committed)
}

/*
PayloadBytes returns txo payload bytes (bytes signed by bearer).
This hack is needed until AB-1012 gets resolved.
*/
func (*tokenExecEnv) PayloadBytes(txo *types.TransactionOrder) ([]byte, error) {
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
	case PayloadTypeCreateNFTType:
		return &CreateNonFungibleTokenTypeAttributes{}, nil
	case PayloadTypeMintNFT:
		return &MintNonFungibleTokenAttributes{}, nil
	case PayloadTypeTransferNFT:
		return &TransferNonFungibleTokenAttributes{}, nil
	case PayloadTypeUpdateNFT:
		return &UpdateNonFungibleTokenAttributes{}, nil
	case PayloadTypeCreateFungibleTokenType:
		return &CreateFungibleTokenTypeAttributes{}, nil
	case PayloadTypeMintFungibleToken:
		return &MintFungibleTokenAttributes{}, nil
	case PayloadTypeTransferFungibleToken:
		return &TransferFungibleTokenAttributes{}, nil
	case PayloadTypeSplitFungibleToken:
		return &SplitFungibleTokenAttributes{}, nil
	case PayloadTypeBurnFungibleToken:
		return &BurnFungibleTokenAttributes{}, nil
	case PayloadTypeJoinFungibleToken:
		return &JoinFungibleTokenAttributes{}, nil
	case PayloadTypeLockToken:
		return &LockTokenAttributes{}, nil
	case PayloadTypeUnlockToken:
		return &UnlockTokenAttributes{}, nil
	}
	return nil, fmt.Errorf("unknown payload type %q", payloadType)
}
