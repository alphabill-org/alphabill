package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/partition"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"github.com/stretchr/testify/require"
)

type (
	CertificateValidator struct{}

	TransactionValidator struct{}
)

var (
	txSystem             = &partition.MockTxSystem{}
	certificateValidator = &CertificateValidator{}
	txValidator          = &TransactionValidator{}
	testConf             = &Configuration{
		SystemIdentifier: nil,
		T1Timeout:        1 * time.Second,
		HashAlgorithm:    gocrypto.SHA256,
		Genesis: &Genesis{
			InputRecord: &certificates.InputRecord{
				PreviousHash: nil,
				Hash:         nil,
				BlockHash:    nil,
				SummaryValue: nil,
			},
			UnicityCertificateRecord: &UnicityCertificate{
				InputRecord: &certificates.InputRecord{
					PreviousHash: []byte{0x1},
					Hash:         []byte{0x2},
					BlockHash:    []byte{0x3},
					SummaryValue: partition.Uint64SummaryValue(12).Bytes(),
				},
				UnicityTreeCertificate: nil,
				UnicityCertificate: &UnicitySeal{
					RootChainBlockNumber: 1,
					PreviousHash:         nil,
					Hash:                 nil,
				},
			},
		},
	}
)

func TestPartition_StartNewRoundCallsRInit(t *testing.T) {
	s := &partition.MockTxSystem{}
	p := newTestPartition(t, s)
	defer p.Close()
	ucr := &UnicityCertificate{
		UnicityCertificate: &UnicitySeal{
			RootChainBlockNumber: 0,
			PreviousHash:         nil,
			Hash:                 nil,
		},
	}
	p.startNewRound(ucr)
	require.Equal(t, uint64(2), s.RoundInitCount)
}

func TestNewPartition_NilInputParameters(t *testing.T) {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	type args struct {
		ctx            context.Context
		txSystem       TransactionSystem
		eb             *eventbus.EventBus
		leaderSelector *LeaderSelector
		ucrValidator   UnicityCertificateRecordValidator
		txValidator    TxValidator
		configuration  *Configuration
	}
	tests := []struct {
		name string
		args args
		err  error
	}{
		{
			"ctx is nil",
			args{
				ctx:            nil,
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConf,
			},
			ErrCtxIsNil,
		},
		{
			"tx system is nil",
			args{
				ctx:            context.Background(),
				txSystem:       nil,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConf,
			},
			ErrTxSystemIsNil,
		},
		{
			"eventbus is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             nil,
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConf,
			},
			ErrEventBusIsNil,
		},
		{
			"leader selector is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: nil,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  testConf,
			},
			ErrLeaderSelectorIsNil,
		},
		{
			"certificate validator is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   nil,
				txValidator:    txValidator,
				configuration:  testConf,
			},
			ErrUcrValidatorIsNil,
		},
		{
			"tx validator is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    nil,
				configuration:  testConf,
			},
			ErrTxValidatorIsNil,
		},
		{
			"configuration is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  nil,
			},
			ErrPartitionConfigurationIsNil,
		},
		{
			"genesis is nil",
			args{
				ctx:            context.Background(),
				txSystem:       txSystem,
				eb:             eventbus.New(),
				leaderSelector: selector,
				ucrValidator:   certificateValidator,
				txValidator:    txValidator,
				configuration:  &Configuration{},
			},
			ErrGenesisIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.ctx, tt.args.txSystem, tt.args.eb, tt.args.leaderSelector, tt.args.ucrValidator, tt.args.txValidator, tt.args.configuration)
			require.ErrorIs(t, err, tt.err)
			require.Nil(t, got)
		})
	}
}

func TestNew_StartsMainLoop(t *testing.T) {
	ctx := context.Background()

	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	eventBus := eventbus.New()
	p, err := New(ctx, txSystem, eventBus, selector, certificateValidator, txValidator, testConf)
	defer p.Close()
	require.NoError(t, err)
	require.Equal(t, idle, p.status)

	pc10, err := eventBus.Subscribe(TopicPC10, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(TopicP1, 10)
	require.NoError(t, err)

	eventBus.Submit(TopicPartitionTransaction, TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
}

func TestPartition_HandleInvalidTxEvent(t *testing.T) {
	ctx := context.Background()

	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	eventBus := eventbus.New()
	p, err := New(ctx, txSystem, eventBus, selector, certificateValidator, txValidator, testConf)
	defer p.Close()
	pc10, err := eventBus.Subscribe(TopicPC10, 10)
	p1, err := eventBus.Subscribe(TopicP1, 10)

	eventBus.Submit(TopicPartitionTransaction, "invalid tx")

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	require.Equal(t, 0, len(p.proposal))
	require.Equal(t, 0, len(p.pr.Transactions))
}

func TestPartition_HandleUnicityCertificateRecordEvent(t *testing.T) {
	ctx := context.Background()

	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	eventBus := eventbus.New()
	p, err := New(ctx, txSystem, eventBus, selector, certificateValidator, txValidator, testConf)
	defer p.Close()
	pc10, err := eventBus.Subscribe(TopicPC10, 10)
	p1, err := eventBus.Subscribe(TopicP1, 10)

	eventBus.Submit(TopicPartitionTransaction, TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	var p1Event P1Event
	require.Eventually(t, func() bool { e := <-p1; p1Event = e.(P1Event); return true }, test.WaitDuration, test.WaitTick)
	err = eventBus.Submit(TopicPartitionUnicityCertificate, UnicityCertificateRecordEvent{
		Certificate: &UnicityCertificate{
			InputRecord:            p1Event.inputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicityCertificate: &UnicitySeal{
				RootChainBlockNumber: 2,
				PreviousHash:         nil,
				Hash:                 nil,
			},
		}},
	)

	require.NoError(t, err)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	require.Equal(t, uint64(2), p.blockStore.Height())
	latestBlock := p.blockStore.LatestBlock()
	require.Equal(t, uint64(2), latestBlock.txSystemBlockNumber)
	require.Equal(t, 1, len(latestBlock.transactions))
}

func createPeer(t *testing.T) *network.Peer {
	ctx := context.Background()
	conf := &network.PeerConfiguration{}

	peer, err := network.NewPeer(ctx, conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func newTestPartition(t *testing.T, txSystem TransactionSystem) *Partition {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	ctx := context.Background()
	p, err := New(ctx, txSystem, eventbus.New(), selector, certificateValidator, txValidator, testConf)
	require.NoError(t, err)
	return p
}

func (c *CertificateValidator) Validate(_ *UnicityCertificate) error {
	return nil
}

func (tv *TransactionValidator) Validate(_ *transaction.Transaction) error {
	return nil
}
