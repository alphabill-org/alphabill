package instrument

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
)

//go:embed add_one.wasm
var addOneWasm []byte

//go:embed conf_tickets.wasm
var ticketWasm []byte

var invalidWasm = []byte("test")

func TestInstrumentWasm(t *testing.T) {
	t.Run("invalid module", func(t *testing.T) {
		// no stack height metering
		res, err := MeterGasAndStack(invalidWasm, 0)
		require.EqualError(t, err, "Invalid magic number at start of file")
		require.Nil(t, res)
	})
	t.Run("ok", func(t *testing.T) {
		// no stack height metering
		res, err := MeterGasAndStack(addOneWasm, 0)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotEqual(t, len(addOneWasm), len(res))
	})
}

func Benchmark(b *testing.B) {
	for i := 0; i < b.N; i++ {
		res, err := MeterGasAndStack(ticketWasm, 1024)
		if err != nil {
			b.Errorf("instrumentation returned error: %v", err)
		}
		if len(res) == len(ticketWasm) {
			b.Errorf("instrumentation result is same as original")
		}
	}
}
