package partition

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils/peer"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/stretchr/testify/require"
)

func Test_loadAndValidateConfiguration_Nok(t *testing.T) {
	signer, _ := testsig.CreateSignerAndVerifier(t)

	type args struct {
		signer  crypto.Signer
		genesis *genesis.PartitionGenesis
		txs     txsystem.TransactionSystem
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "signer is nil",
			args: args{
				signer: nil,
			},
			wantErr: ErrSignerIsNil,
		},
		{
			name: "genesis is nil",
			args: args{
				signer:  signer,
				genesis: nil,
			},
			wantErr: ErrGenesisIsNil,
		},
		{
			name: "transaction system is nil",
			args: args{
				signer:  signer,
				genesis: &genesis.PartitionGenesis{},
				txs:     nil,
			},
			wantErr: ErrTxSystemIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trustBase := &types.RootTrustBaseV1{}
			c, err := loadAndValidateConfiguration(tt.args.signer, tt.args.genesis, trustBase, tt.args.txs)
			require.ErrorIs(t, tt.wantErr, err)
			require.Nil(t, c)
		})
	}
}

func TestLoadConfigurationWithDefaultValues_Ok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	rg, pg := createPartitionGenesis(t, signer, nil, peerConf)

	trustBase, err := types.NewTrustBaseGenesis(rg.Root.RootValidators, pg.Certificate.UnicitySeal.Hash)
	require.NoError(t, err)

	conf, err := loadAndValidateConfiguration(signer, pg, trustBase, &testtxsystem.CounterTxSystem{})

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.NotNil(t, conf.blockStore)
	require.NotNil(t, conf.signer)
	require.NotNil(t, conf.txValidator)
	require.NotNil(t, conf.blockProposalValidator)
	require.NotNil(t, conf.unicityCertificateValidator)
	require.NotNil(t, conf.genesis)
	require.NotNil(t, conf.hashAlgorithm)
	require.Equal(t, DefaultT1Timeout, conf.t1Timeout)
	require.Equal(t, DefaultReplicationMaxBlocks, conf.replicationConfig.maxFetchBlocks)
	require.Equal(t, DefaultReplicationMaxBlocks, conf.replicationConfig.maxReturnBlocks)
	require.Equal(t, DefaultReplicationMaxTx, conf.replicationConfig.maxTx)
	require.Equal(t, DefaultLedgerReplicationTimeout, conf.replicationConfig.timeout)
	require.Equal(t, DefaultBlockSubscriptionTimeout, conf.blockSubscriptionTimeout)
}

func TestLoadConfigurationWithOptions_Ok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, _ := testsig.CreateSignerAndVerifier(t)

	blockStore, err := memorydb.New()
	require.NoError(t, err)
	shardStore, err := memorydb.New()
	require.NoError(t, err)

	t1Timeout := 250 * time.Millisecond
	rg, pg := createPartitionGenesis(t, signer, nil, peerConf)
	trustBase, err := types.NewTrustBaseGenesis(rg.Root.RootValidators, pg.Certificate.UnicitySeal.Hash)
	require.NoError(t, err)

	conf, err := loadAndValidateConfiguration(signer, pg, trustBase, &testtxsystem.CounterTxSystem{},
		WithTxValidator(&AlwaysValidTransactionValidator{}),
		WithUnicityCertificateValidator(&AlwaysValidCertificateValidator{}),
		WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		WithBlockStore(blockStore),
		WithShardStore(shardStore),
		WithT1Timeout(t1Timeout),
		WithReplicationParams(1, 2, 3, 1000),
		WithBlockSubscriptionTimeout(3500),
	)

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.Equal(t, blockStore, conf.blockStore)
	require.Equal(t, shardStore, conf.shardStore)
	require.NoError(t, conf.txValidator.Validate(nil, 0))
	require.NoError(t, conf.blockProposalValidator.Validate(nil, nil))
	require.NoError(t, conf.unicityCertificateValidator.Validate(nil))
	require.Equal(t, t1Timeout, conf.t1Timeout)
	require.EqualValues(t, 1, conf.replicationConfig.maxFetchBlocks)
	require.EqualValues(t, 2, conf.replicationConfig.maxReturnBlocks)
	require.EqualValues(t, 3, conf.replicationConfig.maxTx)
	require.EqualValues(t, 1000, conf.replicationConfig.timeout)
	require.EqualValues(t, 3500, conf.blockSubscriptionTimeout)
}

func createPartitionGenesis(t *testing.T, nodeSigningKey crypto.Signer, rootSigner crypto.Signer, peerConf *network.PeerConfiguration) (*genesis.RootGenesis, *genesis.PartitionGenesis) {
	t.Helper()
	if rootSigner == nil {
		rootSigner, _ = testsig.CreateSignerAndVerifier(t)
	}
	pdr := types.PartitionDescriptionRecord{
		Version:     1,
		NetworkID:   5,
		PartitionID: 0x01000001,
		TypeIDLen:   8,
		UnitIDLen:   256,
		T2Timeout:   2500 * time.Millisecond,
	}
	pn := createPartitionNode(t, nodeSigningKey, pdr, peerConf.ID)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	nodeID, err := network.NodeIDFromPublicKeyBytes(rootPubKeyBytes)
	require.NoError(t, err)
	rg, pg, err := rootgenesis.NewRootGenesis(nodeID.String(), rootSigner, []*genesis.PartitionNode{pn})
	require.NoError(t, err)
	return rg, pg[0]
}

func TestGetRootNodes(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, _ := testsig.CreateSignerAndVerifier(t)

	rg, pg := createPartitionGenesis(t, signer, nil, peerConf)
	trustBase, err := types.NewTrustBaseGenesis(rg.Root.RootValidators, pg.Certificate.UnicitySeal.Hash)
	require.NoError(t, err)

	conf, err := loadAndValidateConfiguration(signer, pg, trustBase, &testtxsystem.CounterTxSystem{})
	require.NoError(t, err)
	nodes, err := conf.getRootNodes()
	require.NoError(t, err)
	require.Len(t, nodes, 1)
}
