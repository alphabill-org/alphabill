package genesis

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootGenesis_IsValid(t *testing.T) {
	var rg *RootGenesis = nil
	err := rg.IsValid(nil, crypto.SHA256)
	require.ErrorIs(t, err, ErrRootGenesisIsNil)
}
