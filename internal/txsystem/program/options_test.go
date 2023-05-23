package program

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/stretchr/testify/require"
)

func TestOptions(t *testing.T) {
	stateSHA512, err := rma.New(&rma.Config{HashAlgorithm: crypto.SHA512})
	require.NoError(t, err)
	tests := []struct {
		name string
		args []Option
		want Options
	}{
		{
			name: "override default values",
			args: []Option{WithHashAlgorithm(crypto.SHA512), WithState(stateSHA512)},
			want: Options{
				hashAlgorithm: crypto.SHA512,
				state:         stateSHA512,
			},
		},
		{
			name: "default values",
			args: []Option{},
			want: Options{
				hashAlgorithm: crypto.SHA256,
				state:         rma.NewWithSHA256(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := defaultOptions()
			for _, arg := range tt.args {
				arg(options)
			}
			require.Equal(t, tt.want.hashAlgorithm, options.hashAlgorithm)
			require.Equal(t, tt.want.state, options.state)
		})
	}
}
