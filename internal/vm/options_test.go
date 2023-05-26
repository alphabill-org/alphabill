package vm

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero"
)

func TestDefault(t *testing.T) {
	options := defaultOptions()
	require.NotNil(t, options.storage)
}

func TestOverrideWazeroCfg(t *testing.T) {
	var args = []Option{WithRuntimeConfig(wazero.NewRuntimeConfig().WithCloseOnContextDone(false).WithMemoryLimitPages(20))}
	options := defaultOptions()
	for _, arg := range args {
		arg(options)
	}
	// There seems to be no good way to check configuration settings applied
}

func TestOverrideStorage(t *testing.T) {
	// just to prove it can be done
	var args = []Option{WithStorage(nil)}
	options := defaultOptions()
	for _, arg := range args {
		arg(options)
	}
	require.Nil(t, options.storage)
}
