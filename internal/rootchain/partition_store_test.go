package rootchain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
)

var id1 = []byte{0, 0, 0, 1}
var id2 = []byte{0, 0, 0, 2}

func TestPartitionStore(t *testing.T) {
	counts := make(map[string]int)
	counts[string(id1)] = 0
	counts[string(id2)] = 2

	type args struct {
		partitions []*genesis.PartitionRecord
	}
	type want struct {
		size                     int
		nodeCounts               map[string]int
		containsPartitions       []string
		doesNotContainPartitions []string
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
					Validators: []*genesis.PartitionNode{nil, nil},
				},
			}},
			want: want{
				size:                     2,
				nodeCounts:               counts,
				containsPartitions:       []string{string(id1), string(id2)},
				doesNotContainPartitions: []string{"1"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newPartitionStore(tt.args.partitions)
			require.Equal(t, tt.want.size, store.size())
			for id, count := range tt.want.nodeCounts {
				require.Equal(t, count, store.nodeCount(id))
			}
			for _, id := range tt.want.containsPartitions {
				require.NotNil(t, store.get(id))
			}
			for _, id := range tt.want.doesNotContainPartitions {
				require.Nil(t, store.get(id))
			}
		})
	}
}