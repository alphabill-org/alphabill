package tokens

import (
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

func TestVerifyPredicates(t *testing.T) {
	tests := []*struct {
		name       string
		predicates []types.PredicateBytes
		signatures [][]byte
		err        string
	}{
		{
			name:       "no predicates, no signatures",
			predicates: []types.PredicateBytes{},
			signatures: [][]byte{},
		},
		{
			name:       "no predicates, one signature",
			predicates: []types.PredicateBytes{},
			signatures: [][]byte{nil},
		},
		{
			name:       "one predicate, one default signature",
			predicates: []types.PredicateBytes{templates.AlwaysTrueBytes()},
			signatures: [][]byte{nil},
		},
		{
			name:       "one predicate, no signatures",
			predicates: []types.PredicateBytes{templates.AlwaysFalseBytes()},
			signatures: [][]byte{},
			err:        "number of signatures (0) not equal to number of parent predicates (1)",
		},
		{
			name:       "one predicate, one empty signature",
			predicates: []types.PredicateBytes{templates.AlwaysTrueBytes()},
			signatures: [][]byte{{}},
		},
		{
			name:       "two predicates (true and false), two signatures, unsatisfiable",
			predicates: []types.PredicateBytes{templates.AlwaysTrueBytes(), templates.AlwaysFalseBytes()},
			signatures: [][]byte{nil, nil},
			err:        "always false",
		},
		{
			name:       "two predicates, two signatures",
			predicates: []types.PredicateBytes{templates.AlwaysTrueBytes(), templates.AlwaysTrueBytes()},
			signatures: [][]byte{nil, nil},
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
