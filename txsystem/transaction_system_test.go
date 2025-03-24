package txsystem

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/stretchr/testify/require"
)

func TestNewStateSummary(t *testing.T) {
	root := test.RandomBytes(32)
	value := test.RandomBytes(8)
	etHash := test.RandomBytes(8)
	summary := NewStateSummary(root, value, etHash)
	require.Equal(t, root, summary.Root())
	require.Equal(t, value, summary.Summary())
	require.Equal(t, etHash, summary.ETHash())
}
