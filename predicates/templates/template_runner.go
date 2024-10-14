package templates

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/hash"
	sdkpredicates "github.com/alphabill-org/alphabill-go-base/predicates"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates"
)

const (
	P2PKHGasCost       = 1000
	AlwaysTrueGasCost  = 100
	AlwaysFalseGasCost = 100
)

var cborNull = []byte{0xf6}

type TemplateRunner struct{}

func New() TemplateRunner {
	return TemplateRunner{}
}

func (TemplateRunner) ID() uint64 {
	return templates.TemplateStartByte
}

func (TemplateRunner) Execute(_ context.Context, p *sdkpredicates.Predicate, args []byte, sigBytesFn func() ([]byte, error), env predicates.TxContext) (bool, error) {
	if p.Tag != templates.TemplateStartByte {
		return false, fmt.Errorf("expected predicate template tag %d but got %d", templates.TemplateStartByte, p.Tag)
	}
	if len(p.Code) != 1 {
		return false, fmt.Errorf("expected predicate template code length to be 1, got %d", len(p.Code))
	}

	switch p.Code[0] {
	case templates.P2pkh256ID:
		return executeP2PKH256TxAuth(p.Params, args, sigBytesFn, env)
	case templates.AlwaysTrueID:
		return executeAlwaysTrue(p.Params, args, env)
	case templates.AlwaysFalseID:
		return executeAlwaysFalse(p.Params, args, env)
	default:
		return false, fmt.Errorf("unknown predicate template with id %d", p.Code[0])
	}
}

func executeAlwaysTrue(params, args []byte, env predicates.TxContext) (bool, error) {
	if err := env.SpendGas(AlwaysTrueGasCost); err != nil {
		return false, err
	}
	// do not allow to piggyback any additional data on "always true" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return true, nil
	}

	return false, fmt.Errorf(`"always true" predicate arguments must be empty`)
}

func executeAlwaysFalse(params, args []byte, env predicates.TxContext) (bool, error) {
	if err := env.SpendGas(AlwaysFalseGasCost); err != nil {
		return false, err
	}
	// do not allow to piggyback any additional data on "always false" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return false, nil
	}

	return false, fmt.Errorf(`"always false" predicate arguments must be empty`)
}

func executeP2PKH256TxAuth(pubKeyHash, args []byte, sigBytesFn func() ([]byte, error), env predicates.TxContext) (bool, error) {
	sigBytes, err := sigBytesFn()
	if err != nil {
		return false, fmt.Errorf("reading transaction sig bytes: %w", err)
	}
	return executeP2PKH256(pubKeyHash, args, sigBytes, env)
}

func executeP2PKH256(pubKeyHash, args []byte, sigBytes []byte, env predicates.TxContext) (bool, error) {
	if err := env.SpendGas(P2PKHGasCost); err != nil {
		return false, err
	}
	p2pkh256Signature := templates.P2pkh256Signature{}
	if err := types.Cbor.Unmarshal(args, &p2pkh256Signature); err != nil {
		return false, fmt.Errorf("failed to decode P2PKH256 signature: %w", err)
	}
	if len(pubKeyHash) != 32 {
		return false, fmt.Errorf("invalid pubkey hash size: expected 32, got %d (%X)", len(pubKeyHash), pubKeyHash)
	}
	if len(p2pkh256Signature.Sig) != 65 {
		return false, fmt.Errorf("invalid signature size: expected 65, got %d (%X)", len(p2pkh256Signature.Sig), p2pkh256Signature.Sig)
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		return false, fmt.Errorf("invalid pubkey size: expected 33, got %d (%X)", len(p2pkh256Signature.PubKey), p2pkh256Signature.PubKey)
	}
	if !bytes.Equal(pubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
		return false, nil
	}

	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		return false, fmt.Errorf("failed to create verifier: %w", err)
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, sigBytes); err != nil {
		if errors.Is(err, crypto.ErrVerificationFailed) {
			return false, nil
		}
		return false, fmt.Errorf("failed to verify signature: %w", err)
	}
	return true, nil
}
