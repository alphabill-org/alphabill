package templates

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

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

type Observability interface {
	Meter(name string, opts ...metric.MeterOption) metric.Meter
}

type TemplateRunner struct {
	execDur metric.Float64Histogram
}

func New(obs Observability) (TemplateRunner, error) {
	m := obs.Meter("predicates.template")
	execDur, err := m.Float64Histogram("exec.time",
		metric.WithDescription("How long it took to execute an predicate"),
		metric.WithUnit("s"),
		// expecting "always tre/false" to be around 4ns (4e-9) and p2pkh 46µs (46e-6)
		metric.WithExplicitBucketBoundaries(3e-9, 4e-9, 5e-9, 45e-6, 46e-6, 47e-6, 48e-6, 49e-6))
	if err != nil {
		return TemplateRunner{}, fmt.Errorf("creating histogram for predicate execution time: %w", err)
	}
	return TemplateRunner{execDur: execDur}, nil
}

func (TemplateRunner) ID() uint64 {
	return templates.TemplateStartByte
}

func (tr TemplateRunner) Execute(ctx context.Context, p *sdkpredicates.Predicate, args []byte, sigBytesFn func() ([]byte, error), env predicates.TxContext) (bool, error) {
	if p.Tag != templates.TemplateStartByte {
		return false, fmt.Errorf("expected predicate template tag %d but got %d", templates.TemplateStartByte, p.Tag)
	}
	if len(p.Code) != 1 {
		return false, fmt.Errorf("expected predicate template code length to be 1, got %d", len(p.Code))
	}
	defer func(start time.Time) {
		tr.execDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributeSet(attribute.NewSet(attribute.Int("template", int(p.Code[0])))))
	}(time.Now())

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
