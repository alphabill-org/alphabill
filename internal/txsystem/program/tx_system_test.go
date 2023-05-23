package program

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name             string
		systemIdentifier []byte
		opts             []Option
		wantErrorStr     string
	}{
		{
			name:             "use default options",
			systemIdentifier: DefaultSmartContractSystemIdentifier,
		},
		{
			name:             "use 'nil' value for system identifier",
			systemIdentifier: nil,
			wantErrorStr:     "system identifier is nil",
		},
		{
			name:             "state is nil",
			systemIdentifier: DefaultSmartContractSystemIdentifier,
			opts:             []Option{WithState(nil)},
			wantErrorStr:     "state is nil",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			got, err := New(ctx, tt.systemIdentifier, tt.opts...)
			if tt.wantErrorStr != "" {
				require.ErrorContains(t, err, tt.wantErrorStr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}
