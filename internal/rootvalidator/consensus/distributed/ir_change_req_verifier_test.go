package distributed

import (
	"reflect"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed/storage"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partitions"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

type (
	MockState struct {
	}
)

func (s *MockState) GetCertificates() map[protocol.SystemIdentifier]*certificates.UnicityCertificate {
	return nil
}

func (s *MockState) IsChangeInProgress(id protocol.SystemIdentifier) bool {
	if id == "inprogress" {
		return true
	}
	return false
}

func TestIRChangeReqVerifier_VerifyIRChangeReq(t *testing.T) {
	type fields struct {
		params        *consensus.Parameters
		state         State
		partitionInfo partitions.PartitionConfiguration
	}
	type args struct {
		round   uint64
		irChReq *atomic_broadcast.IRChangeReqMsg
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    *storage.InputData
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqVerifier{
				params:     tt.fields.params,
				state:      tt.fields.state,
				partitions: tt.fields.partitionInfo,
			}
			got, err := x.VerifyIRChangeReq(tt.args.round, tt.args.irChReq)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyIRChangeReq() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VerifyIRChangeReq() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewIRChangeReqVerifier(t *testing.T) {
	type args struct {
		c        *consensus.Parameters
		pInfo    partitions.PartitionConfiguration
		sMonitor State
	}
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: []byte{0, 0, 0, 1},
				T2Timeout:        2600,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	conf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
	require.NoError(t, err)
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name:       "state monitor is nil",
			args:       args{c: &consensus.Parameters{}, pInfo: conf, sMonitor: nil},
			wantErrStr: "error state monitor is nil",
		},
		{
			name:       "partition store is nil",
			args:       args{c: &consensus.Parameters{}, pInfo: nil, sMonitor: &MockState{}},
			wantErrStr: "error partition store is nil",
		},
		{
			name:       "params is nil",
			args:       args{c: nil, pInfo: conf, sMonitor: &MockState{}},
			wantErrStr: "error consensus params is nil",
		},
		{
			name: "ok",
			args: args{c: &consensus.Parameters{}, pInfo: conf, sMonitor: &MockState{}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewIRChangeReqVerifier(tt.args.c, tt.args.pInfo, tt.args.sMonitor)
			if len(tt.wantErrStr) > 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Nil(t, got)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
		})
	}
}

func TestNewLucBasedT2TimeoutGenerator(t *testing.T) {
	type args struct {
		c        *consensus.Parameters
		pInfo    partitions.PartitionConfiguration
		sMonitor State
	}
	tests := []struct {
		name    string
		args    args
		want    *PartitionTimeoutGenerator
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewLucBasedT2TimeoutGenerator(tt.args.c, tt.args.pInfo, tt.args.sMonitor)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLucBasedT2TimeoutGenerator() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewLucBasedT2TimeoutGenerator() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartitionTimeoutGenerator_GetT2Timeouts(t *testing.T) {
	type fields struct {
		params        *consensus.Parameters
		state         State
		partitionInfo partitions.PartitionConfiguration
	}
	type args struct {
		currenRound uint64
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []protocol.SystemIdentifier
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionTimeoutGenerator{
				params:     tt.fields.params,
				state:      tt.fields.state,
				partitions: tt.fields.partitionInfo,
			}
			if got := x.GetT2Timeouts(tt.args.currenRound); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetT2Timeouts() = %v, want %v", got, tt.want)
			}
		})
	}
}
