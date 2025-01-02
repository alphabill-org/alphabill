package partition

import (
	"slices"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testpeer "github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/stretchr/testify/require"
)

func TestVerifier_Ok(t *testing.T) {
	db, err := memorydb.New()
	require.NoError(t, err)
	ss := newShardStore(db, logger.New(t))

	peer1Conf := testpeer.CreatePeerConfiguration(t)
	peer1Signer, peer1Verifier := testsig.CreateSignerAndVerifier(t)
	_, pg := createPartitionGenesis(t, peer1Signer, nil, peer1Conf)

	vaRec0 := newVARFromGenesis(pg)
	require.NoError(t, ss.StoreValidatorAssignmentRecord(vaRec0))
	require.Nil(t, ss.Verifier(peer1Conf.ID))
	require.EqualValues(t, 0, vaRec0.EpochNumber)
	require.Equal(t, 0, len(ss.Validators()))
	require.NoError(t, ss.LoadEpoch(vaRec0.EpochNumber))
	require.EqualValues(t, 0, ss.LoadedEpoch())
	require.Equal(t, 1, len(ss.Validators()))
	require.Equal(t, peer1Verifier, ss.Verifier(peer1Conf.ID))

	vaRec1 := createVARWithNewNode(t, vaRec0)
	require.NoError(t, ss.StoreValidatorAssignmentRecord(vaRec1))
	require.NoError(t, ss.LoadEpoch(vaRec1.EpochNumber))
	require.EqualValues(t, 1, ss.LoadedEpoch())
	require.Equal(t, 2, len(ss.Validators()))
	require.True(t, ss.IsValidator(peer1Conf.ID))
	require.True(t, slices.Contains(ss.Validators(), ss.RandomValidator()))

	vaRec2 := createVARWithRemovedNode(t, vaRec1, 0)
	require.NoError(t, ss.StoreValidatorAssignmentRecord(vaRec2))
	require.NoError(t, ss.LoadEpoch(vaRec2.EpochNumber))
	require.EqualValues(t, 2, ss.LoadedEpoch())
	require.Equal(t, 1, len(ss.Validators()))
	require.False(t, ss.IsValidator(peer1Conf.ID))
}

func createVARWithNewNode(t *testing.T, prev *partitions.ValidatorAssignmentRecord) *partitions.ValidatorAssignmentRecord {
	peerConf := testpeer.CreatePeerConfiguration(t)
	_, sigVerifier := testsig.CreateSignerAndVerifier(t)
	sigKey, err := sigVerifier.MarshalPublicKey()
	require.NoError(t, err)

	nodes := append(prev.Nodes, &types.NodeInfo{
		NodeID: peerConf.ID.String(),
		SigKey: sigKey,
		Stake:  1,
	})

	return &partitions.ValidatorAssignmentRecord{
		NetworkID:   prev.NetworkID,
		PartitionID: prev.PartitionID,
		ShardID:     prev.ShardID,
		EpochNumber: prev.EpochNumber+1,
		RoundNumber: prev.RoundNumber+100,
		Nodes:       nodes,
	}
}

func createVARWithRemovedNode(t *testing.T, prev *partitions.ValidatorAssignmentRecord, removeNodeIdx uint) *partitions.ValidatorAssignmentRecord {
	nodes := make([]*types.NodeInfo, len(prev.Nodes)-1)
	copy(nodes[0:], prev.Nodes[:removeNodeIdx])
	copy(nodes[removeNodeIdx:], prev.Nodes[removeNodeIdx+1:])

	return &partitions.ValidatorAssignmentRecord{
		NetworkID:   prev.NetworkID,
		PartitionID: prev.PartitionID,
		ShardID:     prev.ShardID,
		EpochNumber: prev.EpochNumber+1,
		RoundNumber: prev.RoundNumber+100,
		Nodes:       nodes,
	}
}
