package partition

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type mockTxSystem struct {
	rinitCount int
}

func (m *mockTxSystem) RInit() {
	m.rinitCount++
}

var txSystem = &mockTxSystem{}
var testConf = &Configuration{PartitionNodeCount: 1}

func TestNew(t *testing.T) {
	type args struct {
		txSystem      TransactionSystem
		configuration *Configuration
	}

	tests := []struct {
		name    string
		args    args
		want    *Partition
		wantErr error
	}{
		{
			name:    "txSystem is nil",
			args:    args{txSystem: nil, configuration: testConf},
			want:    nil,
			wantErr: ErrTxSystemIsNil,
		},
		{
			name:    "configuration is nil",
			args:    args{txSystem: txSystem, configuration: nil},
			want:    nil,
			wantErr: ErrPartitionConfigurationIsNil,
		},
		{
			name: "partition created",
			args: args{txSystem: txSystem, configuration: testConf},
			want: &Partition{
				transactionSystem: txSystem,
				configuration:     testConf,
				l:                 0,
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.txSystem, tt.args.configuration)
			if err != nil || tt.want != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.Equal(t, tt.want.transactionSystem, got.transactionSystem)
			require.Equal(t, tt.want.configuration, got.configuration)
			require.NotNil(t, got.t1)
		})
	}
}

func TestPartition_StartNewRoundCallsRInit(t *testing.T) {
	s := &mockTxSystem{}
	p, _ := New(s, testConf)
	uc := &UnicityCertificate{
		RootChainBlockNumber: 0,
		PreviousHash:         nil,
		Hash:                 nil,
	}
	p.StartNewRound(uc)
	require.Equal(t, 1, s.rinitCount)
}
