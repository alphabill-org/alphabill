package state

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTrustBaseCanBeCreated(t *testing.T) {
	tests := []struct {
		name    string
		hexKeys []string
	}{
		{
			name:    "single key ok",
			hexKeys: []string{"0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"},
		},
		{
			name:    "multiple keys ok",
			hexKeys: []string{"0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb, err := newTrustBase(tt.hexKeys)
			require.NotNil(t, tb)
			require.NoError(t, err)
			require.Len(t, tb.keys, len(tt.hexKeys))
		})
	}
}

func TestTrustBaseErrorConditions(t *testing.T) {
	tests := []struct {
		name    string
		hexKeys []string
		errMsg  string
	}{
		{
			name:    "empty input",
			hexKeys: []string{},
			errMsg:  "unicity trust base must be defined",
		},
		{
			name:    "invalid hex",
			hexKeys: []string{"z212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f1"},
			errMsg:  "error decoding trust base hex key",
		}, {
			name:    "invalid length",
			hexKeys: []string{"000212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f1"},
			errMsg:  "error parsing public key",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tb, err := newTrustBase(tt.hexKeys)
			require.Nil(t, tb)
			require.Error(t, err, tt.errMsg)
		})
	}
}
