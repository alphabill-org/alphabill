package partitions

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
)

func Test_TrustBase_Quorum(t *testing.T) {
	// quorum rule is "50% + 1 node"
	var testCases = []struct {
		count  uint64 // node count
		quorum uint64 // quorum value we expect
	}{
		{count: 1, quorum: 1},
		{count: 2, quorum: 2},
		{count: 3, quorum: 2},
		{count: 4, quorum: 3},
		{count: 5, quorum: 3},
		{count: 6, quorum: 4},
		{count: 99, quorum: 50},
		{count: 100, quorum: 51},
	}
	_, verifier := testsig.CreateSignerAndVerifier(t)
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%d nodes", tc.count), func(t *testing.T) {
			tb := make(map[string]abcrypto.Verifier, tc.count)
			for v := range tc.count {
				tb[fmt.Sprintf("node%d", v)] = verifier
			}
			ptb := NewPartitionTrustBase(tb)
			require.Equal(t, tc.count, ptb.GetTotalNodes())
			require.Equal(t, tc.quorum, ptb.GetQuorum())
		})
	}
}

func Test_TrustBase_Verify(t *testing.T) {
	_, verifier := testsig.CreateSignerAndVerifier(t)
	tb := map[string]abcrypto.Verifier{"node1": verifier}
	ptb := NewPartitionTrustBase(tb)

	t.Run("node not found in the trustbase", func(t *testing.T) {
		err := ptb.Verify("foobar", func(v abcrypto.Verifier) error { return fmt.Errorf("unexpected call") })
		require.EqualError(t, err, `node foobar is not part of partition trustbase`)
	})

	t.Run("message does NOT verify", func(t *testing.T) {
		expErr := fmt.Errorf("nope, thats invalid")
		err := ptb.Verify("node1", func(v abcrypto.Verifier) error { return expErr })
		require.ErrorIs(t, err, expErr)
	})

	t.Run("message does verify", func(t *testing.T) {
		require.NoError(t, ptb.Verify("node1", func(v abcrypto.Verifier) error { return nil }))
	})
}
