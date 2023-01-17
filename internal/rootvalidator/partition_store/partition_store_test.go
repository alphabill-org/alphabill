package partition_store

import (
	"testing"

	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/stretchr/testify/require"
)

var id1 = []byte{0, 0, 0, 1}
var id2 = []byte{0, 0, 0, 2}

func TestPartitionStore(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)

	type args struct {
		partitions []*genesis.GenesisPartitionRecord
	}
	type want struct {
		size                     int
		nodeCounts               []int
		containsPartitions       []p.SystemIdentifier
		doesNotContainPartitions []p.SystemIdentifier
	}
	tests := []struct {
		name string
		args args
		want want
	}{
		{
			name: "create empty store",
			args: args{partitions: nil},
			want: want{
				size:               0,
				nodeCounts:         nil,
				containsPartitions: nil,
			},
		},
		{
			name: "create using an empty array",
			args: args{partitions: []*genesis.GenesisPartitionRecord{}},
			want: want{
				size:               0,
				nodeCounts:         nil,
				containsPartitions: nil,
			},
		},
		{
			name: "create partition store",
			args: args{partitions: []*genesis.GenesisPartitionRecord{
				{
					SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
						SystemIdentifier: id1,
						T2Timeout:        2500,
					},
					Nodes: nil,
				},
				{
					SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
						SystemIdentifier: id2,
						T2Timeout:        2500,
					},
					Nodes: []*genesis.PartitionNode{
						{NodeIdentifier: "test1", SigningPublicKey: pubKeyBytes},
						{NodeIdentifier: "test2", SigningPublicKey: pubKeyBytes},
					},
				}},
			},
			want: want{
				size:                     2,
				nodeCounts:               []int{0, 2},
				containsPartitions:       []p.SystemIdentifier{p.SystemIdentifier(id1), p.SystemIdentifier(id2)},
				doesNotContainPartitions: []p.SystemIdentifier{p.SystemIdentifier("1")},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store, err := NewPartitionStoreFromGenesis(tt.args.partitions)
			require.NoError(t, err)
			require.Equal(t, tt.want.size, len(store.partitions))
			for i, id := range tt.want.containsPartitions {
				info, err := store.Info(id)
				require.NoError(t, err)
				if tt.want.nodeCounts != nil {
					require.Equal(t, tt.want.nodeCounts[i], len(info.TrustBase))
					require.Equal(t, tt.args.partitions[i].SystemDescriptionRecord.SystemIdentifier, info.SystemDescription.GetSystemIdentifier())
					require.Equal(t, tt.args.partitions[i].SystemDescriptionRecord.T2Timeout, info.SystemDescription.GetT2Timeout())
				}
			}
			for _, id := range tt.want.doesNotContainPartitions {
				_, err := store.Info(id)
				require.Error(t, err)
			}
		})
	}
}

// todo: should probably only give read only access
func TestPartitionStore_Info(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	partitions := []*genesis.GenesisPartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: id1,
				T2Timeout:        2600,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "node1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node2", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "node3", SigningPublicKey: pubKeyBytes},
			},
		},
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: id2,
				T2Timeout:        2500,
			},
			Nodes: []*genesis.PartitionNode{
				{NodeIdentifier: "test1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "test2", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	store, err := NewPartitionStoreFromGenesis(partitions)
	require.NoError(t, err)
	info, err := store.Info(p.SystemIdentifier(id1))
	require.NoError(t, err)
	require.Equal(t, id1, info.SystemDescription.SystemIdentifier)
	require.Equal(t, uint32(2600), info.SystemDescription.T2Timeout)
	require.Equal(t, 3, len(info.TrustBase))
	require.Equal(t, uint64(2), info.GetQuorum())
	// try to change the values
	info.SystemDescription.T2Timeout = 100
	// source must not change
	info2, err := store.Info(p.SystemIdentifier(id1))
	require.Equal(t, uint32(100), info2.SystemDescription.T2Timeout)
	require.Equal(t, uint64(2), info2.GetQuorum())
}
