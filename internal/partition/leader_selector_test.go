package partition

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewLeaderSelector_Ok(t *testing.T) {
	ls := NewDefaultLeaderSelector()
	require.NotNil(t, ls)
	require.Equal(t, UnknownLeader, ls.leader.String())
}
