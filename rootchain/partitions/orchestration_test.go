package partitions

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/rootchain/testutils"
	"github.com/stretchr/testify/require"
)

func TestNewOrchestration(t *testing.T) {
	t.Run("directory not exist", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "notExist", "orchestration.db")
		o, err := NewOrchestration(5, dbPath, logger.New(t))
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, o)
	})

	t.Run("ok", func(t *testing.T) {
		// create new orchestration, verify first VAR is created and stored
		dbPath := filepath.Join(t.TempDir(), "orchestration.db")
		o, err := NewOrchestration(5, dbPath, logger.New(t))
		require.NoError(t, err)
		require.NotNil(t, o)
		t.Cleanup(func() { _ = o.db.Close() })

		shardConf := createShardConf(t, 1, types.ShardID{}, 10)
		require.NoError(t, o.AddShardConfig(shardConf))

		shardConfA, err := o.ShardConfig(1, types.ShardID{}, 10)
		require.NoError(t, err)

		require.EqualValues(t, shardConf.NetworkID, shardConfA.NetworkID)
		require.EqualValues(t, shardConf.PartitionID, shardConfA.PartitionID)
		require.EqualValues(t, shardConf.ShardID, shardConfA.ShardID)
		require.EqualValues(t, shardConf.Epoch, shardConfA.Epoch)
		require.EqualValues(t, shardConf.EpochStart, shardConfA.EpochStart)
		validators := shardConfA.Validators
		require.Len(t, validators, 1)
		require.EqualValues(t, shardConf.Validators[0].NodeID, validators[0].NodeID)
		require.EqualValues(t, shardConf.Validators[0].SigKey, validators[0].SigKey)

		// if we now reopen the DB with different genesis file the original
		// data must be preserved and new seed ignored
		require.NoError(t, o.db.Close())
		o, err = NewOrchestration(5, dbPath, logger.New(t))
		require.NoError(t, err)
		require.NotNil(t, o)
		t.Cleanup(func() { _ = o.db.Close() })

		shardConfB, err := o.ShardConfig(1, types.ShardID{}, 10)
		require.NoError(t, err)
		require.Equal(t, shardConfA, shardConfB)
	})
}

func TestShardConfig(t *testing.T) {
	partitionA := types.PartitionID(1)
	partitionB := types.PartitionID(2)
	shardID := types.ShardID{}
	invalidShardID := types.ShardID{}
	err := invalidShardID.UnmarshalText([]byte("0x81"))
	require.NoError(t, err)

	shardConf1 := createShardConf(t, partitionA, shardID, 1)
	shardConf2 := createShardConf(t, partitionB, shardID, 1)

	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(5, dbPath, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })
	require.NoError(t, o.AddShardConfig(shardConf1))
	require.NoError(t, o.AddShardConfig(shardConf2))

	var testCases = []struct {
		partitionID types.PartitionID                 // partition id to query
		shardID     types.ShardID                     // shard id to query
		rootRound   uint64                            // root round to query
		errMsg      string                            // expected err message
		shardConf   *types.PartitionDescriptionRecord // expected shardConf
	}{
		{partitionID: 0, shardID: shardID, rootRound: 1, errMsg: "shard conf missing for shard 00000000_"},
		{partitionID: 3, shardID: shardID, rootRound: 1, errMsg: "shard conf missing for shard 00000003_"},

		{partitionID: 1, shardID: invalidShardID, rootRound: 1, errMsg: "shard conf missing for shard 00000001_1000000"},
		{partitionID: 1, shardID: shardID, rootRound: 0, errMsg: "shard conf missing for shard 00000001_"},
		{partitionID: 1, shardID: shardID, rootRound: 1, shardConf: shardConf1},
		{partitionID: 1, shardID: shardID, rootRound: 999, shardConf: shardConf1},

		{partitionID: 2, shardID: invalidShardID, rootRound: 1, errMsg: "shard conf missing for shard 00000002_1000000"},
		{partitionID: 2, shardID: shardID, rootRound: 0, errMsg: "shard conf missing for shard 00000002_"},
		{partitionID: 2, shardID: shardID, rootRound: 1, shardConf: shardConf2},
		{partitionID: 2, shardID: shardID, rootRound: 888, shardConf: shardConf2},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("query shard conf for partition %q shard %q epoch %q", tc.partitionID, tc.shardID, tc.rootRound), func(t *testing.T) {
			shardConf, err := o.ShardConfig(tc.partitionID, tc.shardID, tc.rootRound)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
				require.Nil(t, shardConf)
			} else {
				require.NoError(t, err)
				require.NotNil(t, shardConf)
				require.Equal(t, tc.shardConf, shardConf)
			}
		})
	}
}

func TestAddShardConfig(t *testing.T) {
	// test adding new config
	// networkID := types.NetworkID(5)
	partitionID := types.PartitionID(1)
	shardID := types.ShardID{}
	invalidShardID := types.ShardID{}
	err := invalidShardID.UnmarshalText([]byte("0x81"))
	require.NoError(t, err)

	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(5, dbPath, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })

	existingShardConf := createShardConf(t, partitionID, shardID, 100)
	require.NoError(t, o.AddShardConfig(existingShardConf))

	var testCases = []struct {
		shardConf *types.PartitionDescriptionRecord // shard conf to add
		errMsg    string                            // expected err message
	}{
		{
			shardConf: func() *types.PartitionDescriptionRecord {
				shardConf := createShardConf(t, partitionID, shardID, 100)
				shardConf.NetworkID = 6
				return shardConf
			}(),
			errMsg: "invalid networkID 6, expected 5",
		},
		{
			shardConf: func() *types.PartitionDescriptionRecord {
				shardConf := createShardConf(t, partitionID, shardID, 200)
				shardConf.Epoch = 2
				return shardConf
			}(),
			errMsg: "verify shard conf: shard conf does not extend previous shard conf: invalid epoch, provided 2 previous 0",
		},
		{
			shardConf: func() *types.PartitionDescriptionRecord {
				// first configuration of a shard should have epoch 0
				shardConf := createShardConf(t, 2, shardID, 200)
				shardConf.Epoch = 1
				return shardConf
			}(),
			errMsg: "verify shard conf: previous shard conf not found",
		},
		{
			shardConf: func() *types.PartitionDescriptionRecord {
				// updating an already added shard conf succeeds, should it not?
				shardConf := createShardConf(t, partitionID, shardID, 100)
				shardConf.T2Timeout = 10 * time.Second
				return shardConf
			}(),
			errMsg: "",
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("add shard config for partition %q shard %q epoch %q", tc.shardConf.PartitionID, tc.shardConf.ShardID, tc.shardConf.Epoch), func(t *testing.T) {
			err := o.AddShardConfig(tc.shardConf)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAddShardConfig_TestAddingShardConfForPreviousEpoch(t *testing.T) {
	// create orchestration
	networkID := types.NetworkID(5)
	partitionID := types.PartitionID(1)
	shardID := types.ShardID{}
	shardConf := createShardConf(t, partitionID, shardID, 1)
	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(5, dbPath, logger.New(t))
	require.NoError(t, err)
	err = o.AddShardConfig(shardConf)
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })

	// add second shardConf
	shardConf2 := &types.PartitionDescriptionRecord{
		NetworkID:   networkID,
		PartitionID: partitionID,
		ShardID:     shardID,
		Epoch:       1,
		EpochStart:  100,
	}
	require.NoError(t, o.AddShardConfig(shardConf2))

	// try to add third shardConf with epoch number same as in the previous shardConf
	shardConf3 := &types.PartitionDescriptionRecord{
		NetworkID:   networkID,
		PartitionID: partitionID,
		ShardID:     shardID,
		Epoch:       1,
		EpochStart:  200,
	}
	require.ErrorContains(t, o.AddShardConfig(shardConf3), "invalid epoch, provided 1 previous 1")
}

func createShardConf(t *testing.T, partitionID types.PartitionID, shardID types.ShardID, epochStart uint64) *types.PartitionDescriptionRecord {
	validator := testutils.NewTestNode(t)
	return &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     partitionID,
		PartitionTypeID: 99,
		ShardID:         shardID,
		UnitIDLen:       256,
		TypeIDLen:       32,
		T2Timeout:       2500 * time.Millisecond,
		Epoch:           0,
		EpochStart:      epochStart,
		Validators:      []*types.NodeInfo{validator.NodeInfo(t)},
	}
}
