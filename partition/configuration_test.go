package partition

import (
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	test "github.com/alphabill-org/alphabill/internal/testutils/peer"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rootgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/stretchr/testify/require"
)

var systemID = []byte{1, 0, 0, 1}

func Test_loadAndValidateConfiguration_Nok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
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
			name: "tx system is nil",
			args: args{
				signer:  signer,
				genesis: createPartitionGenesis(t, signer, verifier, nil, peerConf),
				txs:     nil,
			},
			wantErr: ErrTxSystemIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := loadAndValidateConfiguration(tt.args.signer, tt.args.genesis, tt.args.txs, logger.New(t))
			require.ErrorIs(t, tt.wantErr, err)
			require.Nil(t, c)
		})
	}
}

func TestLoadConfigurationWithDefaultValues_Ok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, peerConf)
	conf, err := loadAndValidateConfiguration(signer, pg, &testtxsystem.CounterTxSystem{}, logger.New(t))

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.NotNil(t, conf.blockStore)
	require.NotNil(t, conf.signer)
	require.NotNil(t, conf.txValidator)
	require.NotNil(t, conf.blockProposalValidator)
	require.NotNil(t, conf.unicityCertificateValidator)
	require.NotNil(t, conf.genesis)
	require.NotNil(t, conf.hashAlgorithm)
	require.NotNil(t, conf.leaderSelector)
	require.Equal(t, DefaultT1Timeout, conf.t1Timeout)
}

func TestLoadConfigurationWithOptions_Ok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	blockStore := memorydb.New()
	selector := NewDefaultLeaderSelector()
	t1Timeout := 250 * time.Millisecond
	pg := createPartitionGenesis(t, signer, verifier, nil, peerConf)
	conf, err := loadAndValidateConfiguration(
		signer,
		pg,
		&testtxsystem.CounterTxSystem{},
		logger.New(t),
		WithTxValidator(&AlwaysValidTransactionValidator{}),
		WithUnicityCertificateValidator(&AlwaysValidCertificateValidator{}),
		WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		WithLeaderSelector(selector),
		WithBlockStore(blockStore),
		WithT1Timeout(t1Timeout),
	)

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.Equal(t, blockStore, conf.blockStore)
	require.NoError(t, conf.txValidator.Validate(nil, 0))
	require.NoError(t, conf.blockProposalValidator.Validate(nil, nil))
	require.NoError(t, conf.unicityCertificateValidator.Validate(nil))
	require.Equal(t, selector, conf.leaderSelector)
	require.Equal(t, t1Timeout, conf.t1Timeout)
}

func createPartitionGenesis(t *testing.T, nodeSigningKey crypto.Signer, nodeEncryptionPubKey crypto.Verifier, rootSigner crypto.Signer, peerConf *network.PeerConfiguration) *genesis.PartitionGenesis {
	t.Helper()
	if rootSigner == nil {
		rootSigner, _ = testsig.CreateSignerAndVerifier(t)
	}
	pn := createPartitionNode(t, nodeSigningKey, nodeEncryptionPubKey, systemID, peerConf.ID)
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	rootPubKeyBytes, err := encPubKey.MarshalPublicKey()
	require.NoError(t, err)
	pr, err := rootgenesis.NewPartitionRecordFromNodes([]*genesis.PartitionNode{pn})
	require.NoError(t, err)
	_, pg, err := rootgenesis.NewRootGenesis("test", rootSigner, rootPubKeyBytes, pr)
	require.NoError(t, err)
	return pg[0]
}

func TestGetPublicKey_Ok(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, peerConf)
	conf, err := loadAndValidateConfiguration(signer, pg, &testtxsystem.CounterTxSystem{}, logger.New(t))
	require.NoError(t, err)

	v, err := conf.GetSigningPublicKey(peerConf.ID.String())
	require.NoError(t, err)
	require.Equal(t, verifier, v)
}

func TestGetPublicKey_NotFound(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	pg := createPartitionGenesis(t, signer, verifier, nil, peerConf)
	conf, err := loadAndValidateConfiguration(signer, pg, &testtxsystem.CounterTxSystem{}, logger.New(t))
	require.NoError(t, err)
	_, err = conf.GetSigningPublicKey("1")
	require.ErrorContains(t, err, "public key for id 1 not found")
}

func TestGetRootNodes(t *testing.T) {
	peerConf := test.CreatePeerConfiguration(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)

	pg := createPartitionGenesis(t, signer, verifier, nil, peerConf)
	conf, err := loadAndValidateConfiguration(signer, pg, &testtxsystem.CounterTxSystem{}, logger.New(t))
	require.NoError(t, err)
	nodes, err := conf.getRootNodes()
	require.NoError(t, err)
	require.Len(t, nodes, 1)
}
