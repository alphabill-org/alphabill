package event

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
)

func TestNewProofIndexer(t *testing.T) {
	type args struct {
		unitProofStorage keyvaluedb.KeyValueDB
		historySize      uint64
		l                *slog.Logger
	}
	tests := []struct {
		name string
		args args
		want *ProofIndexer
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewProofIndexer(tt.args.unitProofStorage, tt.args.historySize, tt.args.l); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewProofIndexer() = %v, want %v", got, tt.want)
			}
		})
	}
}