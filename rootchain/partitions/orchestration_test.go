package partitions

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"math"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/stretchr/testify/require"
)

func TestNewOrchestration(t *testing.T) {
	t.Run("directory not exist", func(t *testing.T) {
		seed := rootGenesis(t, "testdata/root-genesis-A.json")
		dbPath := filepath.Join(t.TempDir(), "notExist", "orchestration.db")
		o, err := NewOrchestration(seed, dbPath)
		require.ErrorIs(t, err, fs.ErrNotExist)
		require.Nil(t, o)
	})

	t.Run("seed is nil", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "orchestration.db")
		o, err := NewOrchestration(nil, dbPath)
		require.ErrorContains(t, err, "root genesis file is required")
		require.Nil(t, o)
	})

	t.Run("ok", func(t *testing.T) {
		// create new orchestration, verify first VAR is created and stored
		rgA := rootGenesis(t, "testdata/root-genesis-A.json")
		dbPath := filepath.Join(t.TempDir(), "orchestration.db")
		o, err := NewOrchestration(rgA, dbPath)
		require.NoError(t, err)
		require.NotNil(t, o)
		t.Cleanup(func() { _ = o.db.Close() })

		recA, err := o.ShardConfig(1, types.ShardID{}, 0)
		require.NoError(t, err)
		pgPartition := rgA.Partitions[0]
		pgPDR := pgPartition.PartitionDescription
		pgIR := pgPartition.Certificate.InputRecord
		require.EqualValues(t, pgPDR.NetworkID, recA.NetworkID)
		require.EqualValues(t, pgPDR.PartitionID, recA.PartitionID)
		require.EqualValues(t, pgPartition.Certificate.ShardTreeCertificate.Shard, recA.ShardID)
		require.EqualValues(t, pgIR.Epoch, recA.EpochNumber)
		require.EqualValues(t, pgIR.RoundNumber, recA.RoundNumber)
		varNodes := recA.Nodes
		require.Len(t, varNodes, 1)
		require.EqualValues(t, pgPartition.Nodes[0].NodeID, varNodes[0].NodeID)
		require.EqualValues(t, pgPartition.Nodes[0].SignKey, varNodes[0].SignKey)
		require.EqualValues(t, pgPartition.Nodes[0].AuthKey, varNodes[0].AuthKey)

		// if we now reopen the DB with different genesis file the original
		// data must be preserved and new seed ignored
		require.NoError(t, o.db.Close())
		rgB := rootGenesis(t, "testdata/root-genesis-B.json")
		o, err = NewOrchestration(rgB, dbPath)
		require.NoError(t, err)
		require.NotNil(t, o)
		t.Cleanup(func() { _ = o.db.Close() })

		recB, err := o.ShardConfig(1, types.ShardID{}, 0)
		require.NoError(t, err)
		require.Equal(t, recA, recB)
	})
}

func TestShardEpoch(t *testing.T) {
	rgA := rootGenesis(t, "testdata/root-genesis-A.json")
	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(rgA, dbPath)
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })

	partitionID := types.PartitionID(1)
	varA, err := o.ShardConfig(partitionID, types.ShardID{}, 0)
	require.NoError(t, err)
	require.EqualValues(t, 1, varA.RoundNumber)

	// epoch 0 starts from round 1
	// add VAR for epoch 1 starting from round 100, and epoch 2 starting from round 200
	varB := &ValidatorAssignmentRecord{
		NetworkID:   varA.NetworkID,
		PartitionID: varA.PartitionID,
		ShardID:     varA.ShardID,
		EpochNumber: 1,
		RoundNumber: 100,
	}
	varC := &ValidatorAssignmentRecord{
		NetworkID:   varA.NetworkID,
		PartitionID: varA.PartitionID,
		ShardID:     varA.ShardID,
		EpochNumber: 2,
		RoundNumber: 200,
	}
	require.NoError(t, o.AddShardConfig(varB))
	require.NoError(t, o.AddShardConfig(varC))

	var testCases = []struct {
		round  uint64 // round number to query
		epoch  uint64 // expected epoch number
		errMsg string // expected err message
	}{
		{round: 0, epoch: 0, errMsg: "epoch not found (db is empty?)"}, // epoch starts from round 1 so round 0 does not belong to any epoch
		{round: 1, epoch: 0},
		{round: 99, epoch: 0},
		{round: 100, epoch: 1},
		{round: 101, epoch: 1},
		{round: 199, epoch: 1},
		{round: 200, epoch: 2},
		{round: 201, epoch: 2},
		{round: 301, epoch: 2},
		{round: math.MaxUint64, epoch: 2},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("query shard epoch for round %d", tc.round), func(t *testing.T) {
			epoch, err := o.ShardEpoch(partitionID, types.ShardID{}, tc.round)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tc.epoch, epoch)
		})
	}
}

func TestShardConfig(t *testing.T) {
	// create multi-partition genesis, attempt to query VARs for different partition and shard combos
	partitionA := types.PartitionID(1)
	partitionB := types.PartitionID(2)
	shardID := types.ShardID{}
	invalidShardID := types.ShardID{}
	err := invalidShardID.UnmarshalText([]byte("0x81"))
	require.NoError(t, err)

	rg := &genesis.RootGenesis{
		Version: 1,
		Partitions: []*genesis.GenesisPartitionRecord{
			createGenesisPartitionRecord(partitionA, shardID),
			createGenesisPartitionRecord(partitionB, shardID),
		},
	}

	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(rg, dbPath)
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })

	var testCases = []struct {
		partitionID types.PartitionID          // partition id to query
		shardID     types.ShardID              // shard id to query
		epoch       uint64                     // epoch number to query
		errMsg      string                     // expected err message
		rec         *ValidatorAssignmentRecord // expected var
	}{
		{partitionID: 0, shardID: shardID, epoch: 0, errMsg: "the partition 0x00000000 does not exist"},
		{partitionID: 3, shardID: shardID, epoch: 0, errMsg: "the partition 0x00000003 does not exist"},

		{partitionID: 1, shardID: invalidShardID, epoch: 0, errMsg: "the partition shard 0x81 does not exist"},
		{partitionID: 1, shardID: shardID, epoch: 1, errMsg: "the epoch 1 does not exist"},
		{partitionID: 1, shardID: shardID, epoch: 0,
			rec: &ValidatorAssignmentRecord{
				NetworkID:   5,
				PartitionID: 1,
				ShardID:     shardID,
				EpochNumber: 0,
				RoundNumber: 1,
				Nodes:       make([]NodeInfo, 0),
			},
		},

		{partitionID: 2, shardID: invalidShardID, epoch: 0, errMsg: "the partition shard 0x81 does not exist"},
		{partitionID: 2, shardID: shardID, epoch: 1, errMsg: "the epoch 1 does not exist"},
		{partitionID: 2, shardID: shardID, epoch: 0,
			rec: &ValidatorAssignmentRecord{
				NetworkID:   5,
				PartitionID: 2,
				ShardID:     shardID,
				EpochNumber: 0,
				RoundNumber: 1,
				Nodes:       make([]NodeInfo, 0),
			}},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("query shard epoch for partition %q shard %q epoch %q", tc.partitionID, tc.shardID, tc.epoch), func(t *testing.T) {
			rec, err := o.ShardConfig(tc.partitionID, tc.shardID, tc.epoch)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
				require.Nil(t, rec)
			} else {
				require.NoError(t, err)
				require.NotNil(t, rec)
				require.Equal(t, tc.rec, rec)
			}
		})
	}
}

func TestAddShardConfig(t *testing.T) {
	// test adding new config
	networkID := types.NetworkID(5)
	partitionID := types.PartitionID(1)
	shardID := types.ShardID{}
	invalidShardID := types.ShardID{}
	err := invalidShardID.UnmarshalText([]byte("0x81"))
	require.NoError(t, err)

	// create orchestration for partition A
	rg := &genesis.RootGenesis{
		Version: 1,
		Partitions: []*genesis.GenesisPartitionRecord{
			createGenesisPartitionRecord(partitionID, shardID),
		},
	}
	dbPath := filepath.Join(t.TempDir(), "orchestration.db")
	o, err := NewOrchestration(rg, dbPath)
	require.NoError(t, err)
	require.NotNil(t, o)
	t.Cleanup(func() { _ = o.db.Close() })

	var testCases = []struct {
		rec    *ValidatorAssignmentRecord // shard config to add
		errMsg string                     // expected err message
	}{
		{
			rec: &ValidatorAssignmentRecord{
				NetworkID:   networkID,
				PartitionID: partitionID,
				ShardID:     shardID,
				EpochNumber: 0,
				RoundNumber: 100,
			},
			errMsg: "invalid epoch number, must not be zero",
		},
		{
			rec: &ValidatorAssignmentRecord{
				NetworkID:   networkID,
				PartitionID: 2,
				ShardID:     shardID,
				EpochNumber: 1,
				RoundNumber: 100,
			},
			errMsg: "the partition 0x00000002 does not exist",
		},
		{
			rec: &ValidatorAssignmentRecord{
				NetworkID:   networkID,
				PartitionID: partitionID,
				ShardID:     invalidShardID,
				EpochNumber: 1,
				RoundNumber: 100,
			},
			errMsg: "the partition shard 0x81 does not exist",
		},
		{
			rec: &ValidatorAssignmentRecord{
				NetworkID:   networkID,
				PartitionID: partitionID,
				ShardID:     shardID,
				EpochNumber: 1,
				RoundNumber: 1,
			},
			errMsg: "invalid shard round number, provided 1 previous 1",
		},
		{
			rec: &ValidatorAssignmentRecord{
				NetworkID:   networkID,
				PartitionID: partitionID,
				ShardID:     shardID,
				EpochNumber: 1,
				RoundNumber: 100,
			},
			errMsg: "", // ok
		},
	}
	for _, tc := range testCases {
		t.Run(fmt.Sprintf("add shard config for partition %q shard %q epoch %q", tc.rec.PartitionID, tc.rec.ShardID, tc.rec.EpochNumber), func(t *testing.T) {
			err := o.AddShardConfig(tc.rec)
			if tc.errMsg != "" {
				require.ErrorContains(t, err, tc.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func createGenesisPartitionRecord(partitionID types.PartitionID, shardID types.ShardID) *genesis.GenesisPartitionRecord {
	return &genesis.GenesisPartitionRecord{
		Version: 1,
		Nodes:   nil,
		Certificate: &types.UnicityCertificate{
			Version:              1,
			ShardTreeCertificate: types.ShardTreeCertificate{Shard: shardID},
			InputRecord:          &types.InputRecord{Epoch: 0, RoundNumber: 1},
		},
		PartitionDescription: &types.PartitionDescriptionRecord{
			Version:     1,
			NetworkID:   5,
			PartitionID: partitionID,
		},
	}
}

func rootGenesis(t *testing.T, path string) *genesis.RootGenesis {
	rgA := &genesis.RootGenesis{Version: 1}
	f, err := genesisFiles.Open(path)
	require.NoError(t, err)
	require.NoError(t, json.NewDecoder(f).Decode(rgA))
	require.NoError(t, f.Close())
	rgA.Verify()
	return rgA
}

//go:embed testdata/*.json
var genesisFiles embed.FS
