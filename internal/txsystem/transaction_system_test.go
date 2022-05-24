package txsystem

import (
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestNewStateSummary(t *testing.T) {
	root := test.RandomBytes(32)
	value := test.RandomBytes(8)
	summary := NewStateSummary(root, value)
	require.Equal(t, root, summary.Root())
	require.Equal(t, value, summary.Summary())
}
