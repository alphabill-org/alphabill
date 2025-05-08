package wvm

import (
	"testing"

	"github.com/tetratelabs/wazero"
)

func TestOverrideWazeroCfg(t *testing.T) {
	var args = []Option{WithRuntimeConfig(wazero.NewRuntimeConfig().WithCloseOnContextDone(false).WithMemoryLimitPages(20))}
	options := defaultOptions()
	for _, arg := range args {
		arg(options)
	}
	// There seems to be no good way to check configuration settings applied
}
