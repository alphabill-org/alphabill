package predicates

import (
	"fmt"

	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

var defaultPredicateRunner PredicateRunner

func RunPredicate(pb types.PredicateBytes, sig []byte, sigData []byte) error {
	return RunPredicateWithContext(pb, &PredicateContext{
		Input:        sig,
		PayloadBytes: sigData,
	})
}

func RunPredicateWithContext(pb types.PredicateBytes, ctx *PredicateContext) error {
	if len(pb) == 0 {
		return fmt.Errorf("predicate is empty")
	}
	if len(pb) > MaxBearerBytes {
		return fmt.Errorf("predicate too large: %d bytes", len(pb))
	}
	if len(ctx.Input) > MaxBearerBytes {
		return fmt.Errorf("predicate signature too large: %d bytes", len(ctx.Input))
	}

	predicate := &Predicate{}
	if err := cbor.Unmarshal(pb, predicate); err != nil {
		return fmt.Errorf("failed to decode predicate '%X': %w", pb, err)
	}

	// In the future if we have multiple runners, predicate.Tag will be used to determine which runner to use.
	if defaultPredicateRunner == nil {
		return fmt.Errorf("no predicate runner registered")
	}
	return defaultPredicateRunner.Execute(predicate, ctx)
}

func RegisterDefaultRunner(runner PredicateRunner) {
	defaultPredicateRunner = runner
}
