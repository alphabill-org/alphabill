package rootvalidator

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
	counts := make(map[p.SystemIdentifier]int)
	counts[p.SystemIdentifier(id1)] = 0
	counts[p.SystemIdentifier(id2)] = 2
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	pubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)

	type args struct {
		partitions []*genesis.PartitionRecord
	}
	type want struct {
		size                     int
		nodeCounts               map[p.SystemIdentifier]int
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
				nodeCounts:               counts,
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
			for id, count := range tt.want.nodeCounts {
				require.Equal(t, count, store.NodeCount(id))
			}
			for _, id := range tt.want.containsPartitions {
				_, err := store.GetNodes(id)
				require.NoError(t, err)

			}
			for _, id := range tt.want.doesNotContainPartitions {
				_, err := store.GetNodes(id)
				require.Error(t, err)
			}
		})
	}
}