package abdrc

import (
	"reflect"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

type (
	MockState struct {
		inProgress []protocol.SystemIdentifier
	}
)

func (s *MockState) GetCertificates() map[protocol.SystemIdentifier]*types.UnicityCertificate {
	return map[protocol.SystemIdentifier]*types.UnicityCertificate{
		protocol.SystemIdentifier(sysID1): {
			InputRecord:            &types.InputRecord{},
			UnicityTreeCertificate: &types.UnicityTreeCertificate{},
			UnicitySeal: &types.UnicitySeal{
				RootChainRoundNumber: 1,
			},
		},
	}
}

func (s *MockState) IsChangeInProgress(id protocol.SystemIdentifier) bool {
	for _, sysId := range s.inProgress {
		if sysId == id {
			return true
		}
	}
	return false
}

func TestIRChangeReqVerifier_VerifyIRChangeReq(t *testing.T) {
	type fields struct {
		c        *consensus.Parameters
		sMonitor State
		pInfo    partitions.PartitionConfiguration
	}
	type args struct {
		round   uint64
		irChReq *abtypes.IRChangeReq
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
		name    string
		fields  fields
		args    args
		want    *storage.InputData
		wantErr bool
	}{
		{
			name: "error change in progress",
			fields: fields{
				c:        &consensus.Parameters{BlockRate: 500 * time.Millisecond},
				pInfo:    conf,
				sMonitor: &MockState{inProgress: []protocol.SystemIdentifier{protocol.SystemIdentifier(sysID1)}}},
			args: args{round: 1, irChReq: &abtypes.IRChangeReq{
				SystemIdentifier: sysID1,
			}},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &IRChangeReqVerifier{
				params:     tt.fields.c,
				state:      tt.fields.sMonitor,
				partitions: tt.fields.pInfo,
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
			},
		},
	}
	conf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
	require.NoError(t, err)
	tests := []struct {
		name       string
		args       args
		want       *PartitionTimeoutGenerator
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
			got, err := NewLucBasedT2TimeoutGenerator(tt.args.c, tt.args.pInfo, tt.args.sMonitor)
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

func TestPartitionTimeoutGenerator_GetT2Timeouts(t *testing.T) {
	type fields struct {
		c        *consensus.Parameters
		sMonitor State
		pInfo    partitions.PartitionConfiguration
	}
	type args struct {
		currentRound uint64
	}
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	genesisPartitions := []*genesis.GenesisPartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: sysID1,
				T2Timeout:        2500,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	conf, err := partitions.NewPartitionStoreFromGenesis(genesisPartitions)
	require.NoError(t, err)
	tests := []struct {
		name   string
		fields fields
		args   args
		want   []protocol.SystemIdentifier
	}{
		{
			name:   "no timeout",
			fields: fields{c: &consensus.Parameters{BlockRate: 500 * time.Millisecond}, pInfo: conf, sMonitor: &MockState{}},
			args:   args{currentRound: 11}, // last certified round is 1 then 11 - 1 = 10 we have not heard from partition in 10 rounds ~ at minimum 2500 ms not yet timeout
			want:   []protocol.SystemIdentifier{},
		},
		{
			name:   "timeout - 6 round since last UC",
			fields: fields{c: &consensus.Parameters{BlockRate: 500 * time.Millisecond}, pInfo: conf, sMonitor: &MockState{}},
			args:   args{currentRound: 12}, // last certified round is 1 then 12 - 1 = 11 we have not heard from partition in 12 rounds ~ at minimum 2750 ms not yet timeout
			want:   []protocol.SystemIdentifier{protocol.SystemIdentifier(sysID1)},
		},
		{
			name: "no timeout - 6 round since last UC, change already in progress",
			fields: fields{
				c:        &consensus.Parameters{BlockRate: 500 * time.Millisecond},
				pInfo:    conf,
				sMonitor: &MockState{inProgress: []protocol.SystemIdentifier{protocol.SystemIdentifier(sysID1)}}},
			args: args{currentRound: 7},
			want: []protocol.SystemIdentifier{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			x := &PartitionTimeoutGenerator{
				params:     tt.fields.c,
				state:      tt.fields.sMonitor,
				partitions: tt.fields.pInfo,
			}
			var tmos []protocol.SystemIdentifier
			tmos, err = x.GetT2Timeouts(tt.args.currentRound)
			require.NoError(t, err)
			require.Equal(t, tt.want, tmos)
		})
	}
}
