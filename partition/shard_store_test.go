package partition

import (
	"slices"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testpeer "github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/stretchr/testify/require"
)

func TestVerifier_Ok(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	ss := newShardStore(db, logger.New(t))

	keyConf, nodeInfo := createKeyConf(t)
	nodeID, err := keyConf.NodeID()
	require.NoError(t, err)
	verifier, err := nodeInfo.SigVerifier()
	require.NoError(t, err)

	shardConf0 := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     0x01020401,
		PartitionTypeID: 1,
		ShardID:         types.ShardID{},
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       800 * time.Millisecond,
		Epoch:           0,
		EpochStart:      0,
		Validators:      []*types.NodeInfo{nodeInfo},
	}
	require.NoError(t, ss.StoreShardConf(shardConf0))
	require.Nil(t, ss.Verifier(nodeID))
	require.Equal(t, 0, len(ss.Validators()))
	require.NoError(t, ss.LoadEpoch(shardConf0.Epoch))
	require.EqualValues(t, 0, ss.LoadedEpoch())
	require.Equal(t, 1, len(ss.Validators()))
	require.Equal(t, verifier, ss.Verifier(nodeID))

	shardConf1 := createShardConfWithNewNode(t, shardConf0)
	require.NoError(t, ss.StoreShardConf(shardConf1))
	require.NoError(t, ss.LoadEpoch(shardConf1.Epoch))
	require.EqualValues(t, 1, ss.LoadedEpoch())
	require.Equal(t, 2, len(ss.Validators()))
	require.True(t, ss.IsValidator(nodeID))
	require.True(t, slices.Contains(ss.Validators(), ss.RandomValidator()))

	shardConf2 := createShardConfWithRemovedNode(t, shardConf1, 0)
	require.NoError(t, ss.StoreShardConf(shardConf2))
	require.NoError(t, ss.LoadEpoch(shardConf2.Epoch))
	require.EqualValues(t, 2, ss.LoadedEpoch())
	require.Equal(t, 1, len(ss.Validators()))
	require.False(t, ss.IsValidator(nodeID))
}

func createShardConfWithNewNode(t *testing.T, prev *types.PartitionDescriptionRecord) *types.PartitionDescriptionRecord {
	peerConf := testpeer.CreatePeerConfiguration(t)
	_, sigVerifier := testsig.CreateSignerAndVerifier(t)
	sigKey, err := sigVerifier.MarshalPublicKey()
	require.NoError(t, err)

	new := *prev
	new.Epoch += 1
	new.EpochStart += 100
	new.Validators = append(prev.Validators, &types.NodeInfo{
		NodeID: peerConf.ID.String(),
		SigKey: sigKey,
		Stake:  1,
	})
	return &new
}

func createShardConfWithRemovedNode(t *testing.T, prev *types.PartitionDescriptionRecord, removeNodeIdx uint) *types.PartitionDescriptionRecord {
	validators := make([]*types.NodeInfo, len(prev.Validators)-1)
	copy(validators[0:], prev.Validators[:removeNodeIdx])
	copy(validators[removeNodeIdx:], prev.Validators[removeNodeIdx+1:])

	new := *prev
	new.Epoch += 1
	new.EpochStart += 100
	new.Validators = validators
	return &new
}
