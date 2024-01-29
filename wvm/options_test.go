package wvm

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

func TestDefault(t *testing.T) {
	options := defaultOptions()
	require.Empty(t, options.hostMod)
}

func TestOverrideWazeroCfg(t *testing.T) {
	var args = []Option{WithRuntimeConfig(wazero.NewRuntimeConfig().WithCloseOnContextDone(false).WithMemoryLimitPages(20))}
	options := defaultOptions()
	for _, arg := range args {
		arg(options)
	}
	// There seems to be no good way to check configuration settings applied
}

func TestAddHostModule(t *testing.T) {
	// just to prove it can be done
	eCtx := TestExecCtx{
		input:  []byte{1, 0, 0, 0, 0, 0, 0, 0},
		params: []byte{2, 0, 0, 0, 0, 0, 0, 0},
	}
	obs := observability.Default(t)
	abHostModuleFn, err := BuildABHostModule(eCtx, obs.Logger(), NewMemoryStorage())
	require.NoError(t, err)
	var args = []Option{WithHostModule(abHostModuleFn)}
	options := defaultOptions()
	for _, arg := range args {
		arg(options)
	}
	require.NotNil(t, options.hostMod)
}
