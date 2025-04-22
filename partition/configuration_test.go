package partition

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/types"
	testobserve "github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
)

func TestNewNodeConf(t *testing.T) {
	blockStore, err := memorydb.New()
	require.NoError(t, err)
	shardStore, err := memorydb.New()
	require.NoError(t, err)
	t1Timeout := 250 * time.Millisecond

	keyConf, nodeInfo := createKeyConf(t)
	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     0x01010101,
		ShardID:         types.ShardID{},
		PartitionTypeID: 999,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		Epoch:           0,
		EpochStart:      1,
		Validators:      []*types.NodeInfo{nodeInfo},
	}
	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := trustbase.NewTrustBase(t, verifier)
	obs := testobserve.Default(t)

	conf, err := NewNodeConf(keyConf, shardConf, trustBase, obs,
		WithTxValidator(&AlwaysValidTransactionValidator{}),
		WithUnicityCertificateValidator(&AlwaysValidCertificateValidator{}),
		WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		WithBlockStore(blockStore),
		WithShardStore(shardStore),
		WithT1Timeout(t1Timeout),
		WithReplicationParams(1, 2, 3, 1000),
		WithBlockSubscriptionTimeout(3500))

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.Equal(t, blockStore, conf.blockStore)
	require.Equal(t, shardStore, conf.shardStore)
	require.NoError(t, conf.txValidator.Validate(nil, 0))
	require.NoError(t, conf.bpValidator.Validate(nil, nil, nil))
	require.NoError(t, conf.ucValidator.Validate(nil, nil))
	require.Equal(t, t1Timeout, conf.t1Timeout)
	require.EqualValues(t, 1, conf.replicationConfig.maxFetchBlocks)
	require.EqualValues(t, 2, conf.replicationConfig.maxReturnBlocks)
	require.EqualValues(t, 3, conf.replicationConfig.maxTx)
	require.EqualValues(t, 1000, conf.replicationConfig.timeout)
	require.EqualValues(t, 3500, conf.blockSubscriptionTimeout)
}

func TestNewNodeConf_WithDefaults(t *testing.T) {
	keyConf, nodeInfo := createKeyConf(t)
	shardConf := &types.PartitionDescriptionRecord{
		Version:         1,
		NetworkID:       5,
		PartitionID:     0x01010101,
		ShardID:         types.ShardID{},
		PartitionTypeID: 999,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		Epoch:           0,
		EpochStart:      1,
		Validators:      []*types.NodeInfo{nodeInfo},
	}

	_, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := trustbase.NewTrustBase(t, verifier)

	obs := testobserve.Default(t)

	_, err := NewNodeConf(nil, shardConf, trustBase, obs)
	require.ErrorIs(t, err, ErrKeyConfIsNil)

	_, err = NewNodeConf(keyConf, nil, trustBase, obs)
	require.ErrorIs(t, err, ErrShardConfIsNil)

	_, err = NewNodeConf(keyConf, shardConf, nil, obs)
	require.ErrorIs(t, err, ErrTrustBaseIsNil)

	conf, err := NewNodeConf(keyConf, shardConf, trustBase, obs)
	require.NoError(t, err)
	require.NotNil(t, conf)

	require.NotNil(t, conf.blockStore)
	require.NotNil(t, conf.signer)
	require.NotNil(t, conf.txValidator)
	require.NotNil(t, conf.bpValidator)
	require.NotNil(t, conf.ucValidator)
	require.NotNil(t, conf.shardConf)
	require.NotNil(t, conf.hashAlgorithm)
	require.Equal(t, DefaultT1Timeout, conf.t1Timeout)
	require.Equal(t, DefaultReplicationMaxBlocks, conf.replicationConfig.maxFetchBlocks)
	require.Equal(t, DefaultReplicationMaxBlocks, conf.replicationConfig.maxReturnBlocks)
	require.Equal(t, DefaultReplicationMaxTx, conf.replicationConfig.maxTx)
	require.Equal(t, DefaultLedgerReplicationTimeout, conf.replicationConfig.timeout)
	require.Equal(t, DefaultBlockSubscriptionTimeout, conf.blockSubscriptionTimeout)

	rootNodes, err := conf.getRootNodes()
	require.NoError(t, err)
	require.Len(t, rootNodes, 1)
}
