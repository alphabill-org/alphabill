package partition_store

import (
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"

	p "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"

	"github.com/stretchr/testify/require"
)

var id1 = []byte{0, 0, 0, 1}
var id2 = []byte{0, 0, 0, 2}

func TestPartitionStore(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)

	type args struct {
		partitions []*genesis.PartitionRecord
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
			args: args{partitions: []*genesis.PartitionRecord{}},
			want: want{
				size:               0,
				nodeCounts:         nil,
				containsPartitions: nil,
			},
		},
		{
			name: "create partition store",
			args: args{partitions: []*genesis.PartitionRecord{
				{
					SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
						SystemIdentifier: id1,
						T2Timeout:        2500,
					},
					Validators: nil,
				},
				{
					SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
						SystemIdentifier: id2,
						T2Timeout:        2500,
					},
					Validators: []*genesis.PartitionNode{
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
			store, err := NewPartitionStore(tt.args.partitions)
			require.NoError(t, err)
			require.Equal(t, tt.want.size, store.Size())
			for i, id := range tt.want.containsPartitions {
				info, err := store.GetInfo(id)
				require.NoError(t, err)
				if tt.want.nodeCounts != nil {
					require.Equal(t, tt.want.nodeCounts[i], len(info.TrustBase))
					require.Equal(t, *tt.args.partitions[i].SystemDescriptionRecord, info.SystemDescription)

				}
			}
			for _, id := range tt.want.doesNotContainPartitions {
				_, err := store.GetInfo(id)
				require.Error(t, err)
			}
		})
	}
}

func TestPartitionStoreAccess(t *testing.T) {
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	partitions := []*genesis.PartitionRecord{
		{
			SystemDescriptionRecord: &genesis.SystemDescriptionRecord{
				SystemIdentifier: id1,
				T2Timeout:        2600,
			},
			Validators: []*genesis.PartitionNode{
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
			Validators: []*genesis.PartitionNode{
				{NodeIdentifier: "test1", SigningPublicKey: pubKeyBytes},
				{NodeIdentifier: "test2", SigningPublicKey: pubKeyBytes},
			},
		},
	}
	store, err := NewPartitionStore(partitions)
	require.NoError(t, err)
	info, err := store.GetInfo(p.SystemIdentifier(id1))
	require.NoError(t, err)
	require.Equal(t, id1, info.SystemDescription.SystemIdentifier)
	require.Equal(t, uint32(2600), info.SystemDescription.T2Timeout)
	require.Equal(t, 3, len(info.TrustBase))
	// try to change the values
	info.SystemDescription.T2Timeout = 100
	// source must not change
	info2, err := store.GetInfo(p.SystemIdentifier(id1))
	require.Equal(t, uint32(2600), info2.SystemDescription.T2Timeout)
}
