package partition

import (
	"context"
	gocrypto "crypto"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"

	testnetwork "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"
	testsig "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/sig"
	testtxsystem "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/txsystem"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/stretchr/testify/require"
)

var systemID = []byte{1, 0, 0, 1}

func Test_loadAndValidateConfiguration_Nok(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	type args struct {
		peer    *network.Peer
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
			name: "peer is nil",
			args: args{
				peer: nil,
			},
			wantErr: ErrPeerIsNil,
		},
		{
			name: "signer is nil",
			args: args{
				peer:   p,
				signer: nil,
			},
			wantErr: ErrSignerIsNil,
		},
		{
			name: "genesis is nil",
			args: args{
				peer:    p,
				signer:  signer,
				genesis: nil,
			},
			wantErr: ErrGenesisIsNil,
		},
		{
			name: "tx system is nil",
			args: args{
				peer:    p,
				signer:  signer,
				genesis: createPartitionGenesis(t, signer, verifier, nil, p),
				txs:     nil,
			},
			wantErr: ErrTxSystemIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := loadAndValidateConfiguration(tt.args.peer, tt.args.signer, tt.args.genesis, tt.args.txs)
			require.ErrorIs(t, tt.wantErr, err)
			require.Nil(t, c)
		})
	}
}

func TestLoadConfigurationWithDefaultValues_Ok(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, p)
	conf, err := loadAndValidateConfiguration(p, signer, pg, &testtxsystem.CounterTxSystem{})

	require.NoError(t, err)
	require.NotNil(t, conf)
	require.NotNil(t, conf.eventbus)
	require.NotNil(t, conf.blockStore)
	require.NotNil(t, conf.signer)
	require.NotNil(t, conf.txValidator)
	require.NotNil(t, conf.blockProposalValidator)
	require.NotNil(t, conf.unicityCertificateValidator)
	require.NotNil(t, conf.genesis)
	require.NotNil(t, conf.hashAlgorithm)
	require.NotNil(t, conf.context)
	require.NotNil(t, conf.leaderSelector)
	require.Equal(t, p, conf.peer)
	require.Equal(t, DefaultT1Timeout, conf.t1Timeout)
}

func TestLoadConfigurationWithOptions_Ok(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	bus := eventbus.New()
	blockStore := &store.InMemoryBlockStore{}
	selector := &mockLeaderSelector{}
	ctx := context.Background()
	t1Timeout := 250 * time.Millisecond
	conf, err := loadAndValidateConfiguration(
		p,
		signer,
		createPartitionGenesis(t, signer, verifier, nil, p),
		&testtxsystem.CounterTxSystem{},
		WithContext(ctx),
		WithEventBus(bus),
		WithTxValidator(&AlwaysValidTransactionValidator{}),
		WithUnicityCertificateValidator(&AlwaysValidCertificateValidator{}),
		WithBlockProposalValidator(&AlwaysValidBlockProposalValidator{}),
		WithLeaderSelector(selector),
		WithBlockStore(blockStore),
		WithT1Timeout(t1Timeout),
	)
	require.NoError(t, err)
	require.NotNil(t, conf)
	require.Equal(t, bus, conf.eventbus)
	require.Equal(t, ctx, conf.context)
	require.Equal(t, blockStore, conf.blockStore)
	require.NoError(t, conf.txValidator.Validate(nil))
	require.NoError(t, conf.blockProposalValidator.Validate(nil, nil))
	require.NoError(t, conf.unicityCertificateValidator.Validate(nil))
	require.Equal(t, selector, conf.leaderSelector)
	require.Equal(t, p, conf.peer)
	require.Equal(t, t1Timeout, conf.t1Timeout)
}

func createPartitionGenesis(t *testing.T, nodeSigningKey crypto.Signer, nodeEncryptionPubKey crypto.Verifier, rootSigner crypto.Signer, p *network.Peer) *genesis.PartitionGenesis {
	t.Helper()
	if rootSigner == nil {
		rootSigner, _ = testsig.CreateSignerAndVerifier(t)
	}
	pn := createPartitionNode(t, nodeSigningKey, nodeEncryptionPubKey, systemID, p.ID())
	_, encPubKey := testsig.CreateSignerAndVerifier(t)
	_, pg, err := rootchain.NewGenesisFromPartitionNodes([]*genesis.PartitionNode{pn}, rootSigner, encPubKey)
	require.NoError(t, err)
	return pg[0]
}

func Test_isGenesisValid_NotOk(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	nodeSigner, nodeVerifier := testsig.CreateSignerAndVerifier(t)
	rootSigner, rootVerifier := testsig.CreateSignerAndVerifier(t)
	type fields struct {
		genesis   *genesis.PartitionGenesis
		trustBase crypto.Verifier
	}
	type args struct {
		txs txsystem.TransactionSystem
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		wantErr string
	}{
		{
			name: "invalid genesis",
			fields: fields{
				genesis:   nil,
				trustBase: rootVerifier,
			},
			args: args{
				txs: &testtxsystem.CounterTxSystem{},
			},
			wantErr: genesis.ErrPartitionGenesisIsNil.Error(),
		},
		{
			name: "invalid genesis input record hash",
			fields: fields{
				genesis:   createPartitionGenesis(t, nodeSigner, nodeVerifier, rootSigner, p),
				trustBase: rootVerifier,
			},
			args: args{
				txs: &testtxsystem.CounterTxSystem{
					InitCount: 100,
				},
			},
			wantErr: "tx system root hash does not equal to genesis file hash",
		},
		{
			name: "invalid genesis summary value",
			fields: fields{
				genesis:   createPartitionGenesis(t, nodeSigner, nodeVerifier, rootSigner, p),
				trustBase: rootVerifier,
			},
			args: args{
				txs: &testtxsystem.CounterTxSystem{
					SummaryValue: 100,
				},
			},
			wantErr: "tx system summary value does not equal to genesis file summary value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &configuration{
				hashAlgorithm: gocrypto.SHA256,
				genesis:       tt.fields.genesis,
				trustBase:     tt.fields.trustBase,
			}
			err := c.isGenesisValid(tt.args.txs)
			require.Error(t, err)
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestGetPublicKey_Ok(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, p)
	conf, err := loadAndValidateConfiguration(p, signer, pg, &testtxsystem.CounterTxSystem{})
	require.NoError(t, err)
	v, err := conf.GetSigningPublicKey(p.ID().String())
	require.NoError(t, err)
	require.Equal(t, verifier, v)
}

func TestGetPublicKey_NotFound(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, p)
	conf, err := loadAndValidateConfiguration(p, signer, pg, &testtxsystem.CounterTxSystem{})
	require.NoError(t, err)
	_, err = conf.GetSigningPublicKey("1")
	require.ErrorContains(t, err, "public key with node id 1 not found")
}

func TestGetGenesisBlock(t *testing.T) {
	p := testnetwork.CreatePeer(t)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pg := createPartitionGenesis(t, signer, verifier, nil, p)
	conf, err := loadAndValidateConfiguration(p, signer, pg, &testtxsystem.CounterTxSystem{})
	require.NoError(t, err)
	require.NotNil(t, conf.genesisBlock())
}

type mockLeaderSelector struct {
}

func (m mockLeaderSelector) LeaderFromUnicitySeal(seal *certificates.UnicitySeal) peer.ID {
	return ""
}

func (m mockLeaderSelector) UpdateLeader(*certificates.UnicitySeal) {
}

func (m mockLeaderSelector) IsCurrentNodeLeader() bool {
	return true
}

func (m mockLeaderSelector) SelfID() peer.ID {
	return ""
}
