package testutils

import (
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/stretchr/testify/require"
)

func NewOrchestration(t *testing.T) *partitions.Orchestration {
	orchestration, err := partitions.NewOrchestration(filepath.Join(t.TempDir(), "orchestration.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = orchestration.Close() })
	return orchestration
}
