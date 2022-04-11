package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rootchain"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"github.com/stretchr/testify/require"
)

var (
	certificateValidator = &CertificateValidator{}
	txValidator          = &TransactionValidator{}
)

type (
	CertificateValidator struct{}

	TransactionValidator struct{}
)

func TestPartition_StartNewRoundCallsRInit(t *testing.T) {
	s := &partition.MockTxSystem{}
	p, _ := createTestPartitionWithTxSystem(t, s, testConfiguration(t, &partition.MockTxSystem{}))
	ucr := &certificates.UnicityCertificate{
		UnicitySeal: &certificates.UnicitySeal{
			RootChainRoundNumber: 0,
			PreviousHash:         nil,
			Hash:                 nil,
		},
	}
	p.startNewRound(ucr)
	require.Equal(t, uint64(2), s.BeginBlockCount)
}

func TestNewPartition_NotOk(t *testing.T) {
	blockStore := NewInMemoryBlockStore()
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	type args struct {
		ctx            context.Context
		txSystem       TransactionSystem
		eb             *eventbus.EventBus
		leaderSelector *LeaderSelector
		ucrValidator   UnicityCertificateValidator
		txValidator    TxValidator
		configuration  *Configuration
		blockStore     BlockStore
	}
	tests := []struct {
		name   string
		args   args
		err    error
		errStr string
	}{
		{
			name: "ctx is nil",
			args: args{
				ctx:            nil,
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration: &Configuration{
					T1Timeout:     700 * time.Millisecond,
					TrustBase:     nil,
					Signer:        nil,
					HashAlgorithm: 0,
					Genesis:       nil},
			},
			err: ErrCtxIsNil,
		},
		{
			name: "tx system is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       nil,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
			},
			err: ErrTxSystemIsNil,
		},
		{
			name: "eventbus is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             nil,
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
			},
			err: ErrEventBusIsNil,
		},
		{
			name: "leader selector is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: nil,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
			},
			err: ErrLeaderSelectorIsNil,
		},
		{
			name: "certificate validator is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   nil,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
			},
			err: ErrUnicityCertificateValidatorIsNil,
		},
		{
			name: "tx validator is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    nil,
				blockStore:     blockStore,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
			},
			err: ErrTxValidatorIsNil,
		},
		{
			name: "configuration is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration:  nil,
			},
			err: ErrPartitionConfigurationIsNil,
		},
		{
			name: "genesis is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration: &Configuration{
					T1Timeout:     2500,
					HashAlgorithm: gocrypto.SHA256,
					Genesis:       nil,
				},
			},
			err: ErrGenesisIsNil,
		},
		{
			name: "invalid partition genesis file",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				blockStore:     blockStore,
				configuration: &Configuration{
					T1Timeout:     2500,
					HashAlgorithm: gocrypto.SHA256,
					Genesis:       &genesis.PartitionGenesis{},
				},
			},
			errStr: "invalid root partition genesis file",
		},
		{
			name: "block store is nil",
			args: args{
				ctx:            context.Background(),
				txSystem:       &partition.MockTxSystem{},
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConfiguration(t, &partition.MockTxSystem{}),
				blockStore:     nil,
			},
			err: ErrBlockStoreIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.ctx, tt.args.txSystem, tt.args.eb, tt.args.leaderSelector, tt.args.ucrValidator, tt.args.txValidator, tt.args.blockStore, tt.args.configuration)
			if tt.err != nil {
				require.ErrorIs(t, err, tt.err)
			} else {
				require.True(t, strings.Contains(err.Error(), tt.errStr))
			}
			require.Nil(t, got)
		})
	}
}

func TestNew_InvalidGenesisRootHash(t *testing.T) {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	conf := testConfiguration(t, &partition.MockTxSystem{})

	system := &partition.MockTxSystem{}
	system.EndBlockCount = 100

	p, err := New(
		context.Background(),
		system,
		eventbus.New(),
		selector,
		certificateValidator,
		txValidator,
		NewInMemoryBlockStore(),
		conf,
	)
	require.Nil(t, p)
	require.ErrorIs(t, err, ErrInvalidRootHash)
}

func TestNew_InvalidSummaryValue(t *testing.T) {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	conf := testConfiguration(t, &partition.MockTxSystem{})

	system := &partition.MockTxSystem{}
	system.SummaryValue = 100

	p, err := New(
		context.Background(),
		system,
		eventbus.New(),
		selector,
		certificateValidator,
		txValidator,
		NewInMemoryBlockStore(),
		conf,
	)
	require.Nil(t, p)
	require.ErrorIs(t, err, ErrInvalidSummaryValue)
}

func TestNew_StartsMainLoop(t *testing.T) {
	_, eventBus := createPartitionWithDefaultTxSystem(t)

	pc10, err := eventBus.Subscribe(TopicPC1O, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(TopicP1, 10)
	require.NoError(t, err)

	eventBus.Submit(TopicPartitionTransaction, TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
}

func TestPartition_HandleInvalidTxEvent(t *testing.T) {
	conf := testConfiguration(t, &partition.MockTxSystem{})
	p, eventBus := createTestPartitionWithTxSystem(t, &partition.MockTxSystem{}, conf)
	pc10, err := eventBus.Subscribe(TopicPC1O, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(TopicP1, 10)

	require.NoError(t, err)
	eventBus.Submit(TopicPartitionTransaction, "invalid tx")

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	require.Equal(t, 0, len(p.proposal))
	require.Equal(t, 0, len(p.pr.Transactions))
}

func TestPartition_HandleUnicityCertificateRecordEvent(t *testing.T) {
	partitionConf := testConfiguration(t, &partition.MockTxSystem{})
	p, eventBus := createTestPartitionWithTxSystem(t, &partition.MockTxSystem{}, partitionConf)
	pc1O, err := eventBus.Subscribe(TopicPC1O, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(TopicP1, 10)
	require.NoError(t, err)

	err = eventBus.Submit(TopicPartitionTransaction, TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})
	require.NoError(t, err)

	require.Eventually(t, func() bool { <-pc1O; return true }, test.WaitDuration, test.WaitTick)
	var p1Event P1Event
	require.Eventually(t, func() bool { e := <-p1; p1Event = e.(P1Event); return true }, test.WaitDuration, test.WaitTick)
	err = eventBus.Submit(TopicPartitionUnicityCertificate, UnicityCertificateEvent{
		Certificate: &certificates.UnicityCertificate{
			InputRecord: p1Event.inputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier: []byte{0, 0, 0, 1},
			},
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: 2,
				PreviousHash:         nil,
				Hash:                 nil,
			},
		}},
	)

	require.NoError(t, err)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	height, err := p.blockStore.Height()
	require.NoError(t, err)
	require.Equal(t, uint64(2), height)

	latestBlock, _ := p.blockStore.LatestBlock()
	require.Equal(t, uint64(2), latestBlock.TxSystemBlockNumber)
	require.Equal(t, 1, len(latestBlock.Transactions))
}

func createPeer(t *testing.T) *network.Peer {
	conf := &network.PeerConfiguration{}
	peer, err := network.NewPeer(conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := libp2pcrypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func createTestPartitionWithTxSystem(t *testing.T, system TransactionSystem, conf *Configuration) (*Partition, *eventbus.EventBus) {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	ctx := context.Background()
	bus := eventbus.New()
	p, err := New(ctx, system, bus, selector, certificateValidator, txValidator, NewInMemoryBlockStore(), conf)
	require.NoError(t, err)
	require.Equal(t, idle, p.status)
	t.Cleanup(p.Close)
	return p, bus
}

func createPartitionWithDefaultTxSystem(t *testing.T) (*Partition, *eventbus.EventBus) {
	conf := testConfiguration(t, &partition.MockTxSystem{})
	return createTestPartitionWithTxSystem(t, &partition.MockTxSystem{}, conf)
}

func testConfiguration(t *testing.T, system TransactionSystem) *Configuration {
	_, partitionGenesis, nodeSigner, rootSigner := initPartitionGenesis(t, system)
	rootVerifier, err := rootSigner.Verifier()
	require.NoError(t, err)
	return &Configuration{
		T1Timeout:     1 * time.Second,
		TrustBase:     rootVerifier,
		Signer:        nodeSigner,
		HashAlgorithm: gocrypto.SHA256,
		Genesis:       partitionGenesis,
	}
}

func initPartitionGenesis(t *testing.T, system TransactionSystem) (*genesis.RootGenesis, *genesis.PartitionGenesis, crypto.Signer, crypto.Signer) {
	t.Helper()
	nodeSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	rootSigner, err := crypto.NewInMemorySecp256K1Signer()
	require.NoError(t, err)

	pn, err := NewNodeGenesis(system, WithPeerID("1"), WithSystemIdentifier([]byte{0, 0, 0, 1}), WithSigner(nodeSigner))
	require.NoError(t, err)

	pr, err := NewPartitionGenesis([]*genesis.PartitionNode{pn}, 2500)
	require.NoError(t, err)

	rootGenesis, pss, err := rootchain.NewGenesis([]*genesis.PartitionRecord{pr}, rootSigner)
	require.NoError(t, err)

	return rootGenesis, pss[0], nodeSigner, rootSigner
}

func (c *CertificateValidator) Validate(_ *certificates.UnicityCertificate) error {
	return nil
}

func (tv *TransactionValidator) Validate(_ *transaction.Transaction) error {
	return nil
}
