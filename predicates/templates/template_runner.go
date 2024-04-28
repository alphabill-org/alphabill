package templates

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/hash"
	sdkpredicates "github.com/alphabill-org/alphabill-go-sdk/predicates"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill/predicates"
)

var cborNull = []byte{0xf6}

type TemplateRunner struct{}

func New() TemplateRunner {
	return TemplateRunner{}
}

func (TemplateRunner) ID() uint64 {
	return templates.TemplateStartByte
}

func (TemplateRunner) Execute(ctx context.Context, p *sdkpredicates.Predicate, args []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
	if p.Tag != templates.TemplateStartByte {
		return false, fmt.Errorf("expected predicate template tag %d but got %d", templates.TemplateStartByte, p.Tag)
	}
	if len(p.Code) != 1 {
		return false, fmt.Errorf("expected predicate template code length to be 1, got %d", len(p.Code))
	}

	switch p.Code[0] {
	case templates.P2pkh256ID:
		return p2pkh256_Execute(p.Params, args, txo, env)
	case templates.AlwaysTrueID:
		return alwaysTrue_Execute(p.Params, args)
	case templates.AlwaysFalseID:
		return alwaysFalse_Execute(p.Params, args)
	default:
		return false, fmt.Errorf("unknown predicate template with id %d", p.Code[0])
	}
}

func alwaysTrue_Execute(params, args []byte) (bool, error) {
	// do not allow to piggyback any additional data on "always true" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return true, nil
	}

	return false, fmt.Errorf(`"always true" predicate arguments must be empty`)
}

func alwaysFalse_Execute(params, args []byte) (bool, error) {
	// do not allow to piggyback any additional data on "always false" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return false, nil
	}

	return false, fmt.Errorf(`"always false" predicate arguments must be empty`)
}

func p2pkh256_Execute(pubKeyHash, sig []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
	// when AB-1012 gets resolved should call txo.PayloadBytes() instead
	payloadBytes, err := env.PayloadBytes(txo)
	if err != nil {
		return false, fmt.Errorf("reading transaction payload bytes: %w", err)
	}

	p2pkh256Signature := templates.P2pkh256Signature{}
	if err := types.Cbor.Unmarshal(sig, &p2pkh256Signature); err != nil {
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
		return false, errors.New("pubkey hash does not match")
	}

	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		return false, fmt.Errorf("failed to create verifier: %w", err)
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, payloadBytes); err != nil {
		return false, fmt.Errorf("failed to verify signature: %w", err)
	}
	return true, nil
}
