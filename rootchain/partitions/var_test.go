package partitions

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

// TODO: move parts to shardConf test
func TestVerify(t *testing.T) {
	invalidShardID := types.ShardID{}
	require.NoError(t, invalidShardID.UnmarshalText([]byte("0x81")))

	node1ID, node1AuthKey := testutils.RandomNodeID(t)

	var testCases = []struct {
		name   string
		prev   *ValidatorAssignmentRecord
		next   *ValidatorAssignmentRecord
		errMsg string
	}{
		{
			name:   "invalid network id",
			prev:   &ValidatorAssignmentRecord{NetworkID: 1},
			next:   &ValidatorAssignmentRecord{NetworkID: 2},
			errMsg: "invalid network id, provided 2 previous 1",
		},
		{
			name:   "invalid partition id",
			prev:   &ValidatorAssignmentRecord{PartitionID: 1},
			next:   &ValidatorAssignmentRecord{PartitionID: 2},
			errMsg: "invalid partition id, provided 2 previous 1",
		},
		{
			name:   "invalid shard id",
			prev:   &ValidatorAssignmentRecord{ShardID: types.ShardID{}},
			next:   &ValidatorAssignmentRecord{ShardID: invalidShardID},
			errMsg: `invalid shard id, provided "0x81" previous "0x80"`,
		},
		{
			name:   "invalid epoch number (next smaller than curr)",
			prev:   &ValidatorAssignmentRecord{EpochNumber: 1},
			next:   &ValidatorAssignmentRecord{EpochNumber: 0},
			errMsg: "invalid epoch number, provided 0 previous 1",
		},
		{
			name:   "invalid epoch number (next equal to curr)",
			prev:   &ValidatorAssignmentRecord{EpochNumber: 1},
			next:   &ValidatorAssignmentRecord{EpochNumber: 1},
			errMsg: "invalid epoch number, provided 1 previous 1",
		},
		{
			name:   "invalid epoch number (next greater than curr by more than 1)",
			prev:   &ValidatorAssignmentRecord{EpochNumber: 1},
			next:   &ValidatorAssignmentRecord{EpochNumber: 3},
			errMsg: "invalid epoch number, provided 3 previous 1",
		},
		{
			name:   "invalid shard round number (next less than curr)",
			prev:   &ValidatorAssignmentRecord{RoundNumber: 1},
			next:   &ValidatorAssignmentRecord{RoundNumber: 0, EpochNumber: 1},
			errMsg: "invalid shard round number, provided 0 previous 1",
		},
		{
			name:   "invalid shard round number (next equal to curr)",
			prev:   &ValidatorAssignmentRecord{RoundNumber: 1},
			next:   &ValidatorAssignmentRecord{RoundNumber: 1, EpochNumber: 1},
			errMsg: "invalid shard round number, provided 1 previous 1",
		},
		{
			name: "invalid node sigKey",
			prev: &ValidatorAssignmentRecord{},
			next: &ValidatorAssignmentRecord{RoundNumber: 1, EpochNumber: 1,
				Nodes: []*types.NodeInfo{
					{
						NodeID: node1ID,
						SigKey: []byte{1},
						Stake:  1,
					},
				}},
			errMsg: "invalid node at idx 0: signing key is invalid",
		},
		{
			name: "ok with nodes",
			prev: &ValidatorAssignmentRecord{},
			next: &ValidatorAssignmentRecord{RoundNumber: 1, EpochNumber: 1,
				Nodes: []*types.NodeInfo{
					{
						NodeID: node1ID,
						SigKey: node1AuthKey,
						Stake:  1,
					},
				}},
		},
		{
			name: "ok without nodes",
			prev: &ValidatorAssignmentRecord{},
			next: &ValidatorAssignmentRecord{RoundNumber: 1, EpochNumber: 1},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.next.Verify(tc.prev)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
