package predicates

import (
	"fmt"

	"github.com/fxamacker/cbor/v2"
)

var defaultPredicateRunner PredicateRunner

func RunPredicate(pb PredicateBytes, sig []byte, sigData []byte) error {
	if len(pb) == 0 {
		return fmt.Errorf("predicate is empty")
	}
	if len(pb) > MaxBearerBytes {
		return fmt.Errorf("predicate too large: %d bytes", len(pb))
	}
	if len(sig) > MaxBearerBytes {
		return fmt.Errorf("predicate signature too large: %d bytes", len(sig))
	}

	predicate := &Predicate{}
	if err := cbor.Unmarshal(pb, predicate); err != nil {
		return fmt.Errorf("failed to decode predicate '%X': %w", pb, err)
	}

	// In the future if we have multiple runners, predicate.Tag will be used to determine which runner to use.
	if defaultPredicateRunner == nil {
		return fmt.Errorf("no predicate runner registered")
	}
	return defaultPredicateRunner.Execute(predicate, sig, sigData)
}

func RegisterDefaultRunner(runner PredicateRunner) {
	defaultPredicateRunner = runner
}
