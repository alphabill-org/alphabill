package wvm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
)

func Test_expCurrentTime(t *testing.T) {
	// module is actually not used by the API
	mod := &mockApiMod{}

	t.Run("no UC", func(t *testing.T) {
		vec := &vmContext{
			curPrg: &evalContext{
				env: &mockTxContext{
					committedUC: func() *types.UnicityCertificate {
						return nil
					},
				},
			},
		}
		stack := []uint64{0}

		err := expCurrentTime(vec, mod, stack)
		require.EqualError(t, err, `no committed UC available`)
		require.Zero(t, stack[0])
	})

	t.Run("UC with timestamp", func(t *testing.T) {
		vec := &vmContext{
			curPrg: &evalContext{
				env: &mockTxContext{
					committedUC: func() *types.UnicityCertificate {
						return &types.UnicityCertificate{
							UnicitySeal: &types.UnicitySeal{Timestamp: 1000},
						}
					},
				},
			},
		}
		stack := []uint64{0}

		require.NoError(t, expCurrentTime(vec, mod, stack))
		require.EqualValues(t, 1000, stack[0])
	})
}

func Test_expCurrentRound(t *testing.T) {
	vec := &vmContext{
		curPrg: &evalContext{
			env: &mockTxContext{
				curRound: func() uint64 { return 567 },
			},
		},
	}
	stack := []uint64{0}
	require.NoError(t, expCurrentRound(vec, &mockApiMod{}, stack))
	require.EqualValues(t, 567, stack[0])
}
