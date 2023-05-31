package tokens

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/stretchr/testify/require"
)

func TestVerifyPredicates(t *testing.T) {
	tests := []*struct {
		name       string
		predicates []Predicate
		signatures [][]byte
		err        string
	}{
		{
			name:       "no predicates, no signatures",
			predicates: []Predicate{},
			signatures: [][]byte{},
		},
		{
			name:       "no predicates, one signature",
			predicates: []Predicate{},
			signatures: [][]byte{script.PredicateArgumentEmpty()},
		},
		{
			name:       "one predicate, one default signature",
			predicates: []Predicate{script.PredicateAlwaysTrue()},
			signatures: [][]byte{script.PredicateArgumentEmpty()},
		},
		{
			name:       "one predicate, no signatures",
			predicates: []Predicate{script.PredicateAlwaysFalse()},
			signatures: [][]byte{},
			err:        "number of signatures (0) not equal to number of parent predicates (1)",
		},
		{
			name:       "one predicate, one empty signature",
			predicates: []Predicate{script.PredicateAlwaysTrue()},
			signatures: [][]byte{{}},
			err:        "invalid script format",
		},
		{
			name:       "two predicates (true and false), two signatures, unsatisfiable",
			predicates: []Predicate{script.PredicateAlwaysTrue(), script.PredicateAlwaysFalse()},
			signatures: [][]byte{script.PredicateArgumentEmpty(), script.PredicateArgumentEmpty()},
			err:        "script execution result yielded false",
		},
		{
			name:       "two predicates, two signatures",
			predicates: []Predicate{script.PredicateAlwaysTrue(), script.PredicateAlwaysTrue()},
			signatures: [][]byte{script.PredicateArgumentEmpty(), script.PredicateArgumentEmpty()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifyPredicates(tt.predicates, tt.signatures, nil)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
