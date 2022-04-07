package partition

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"hash"
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"github.com/libp2p/go-libp2p-core/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/state"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/eventbus"

	"github.com/stretchr/testify/require"
)

type (
	mockTxSystem struct {
		rinitCount   uint64
		rComplCount  uint64
		executeCount uint64
	}

	Uint64SummaryValue uint64

	CertificateValidator struct{}

	TransactionValidator struct{}
)

var (
	txSystem             = &mockTxSystem{}
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
			UnicityCertificateRecord: &certificates.UnicityCertificate{
				InputRecord: &certificates.InputRecord{
					PreviousHash: []byte{0x1},
					Hash:         []byte{0x2},
					BlockHash:    []byte{0x3},
					SummaryValue: Uint64SummaryValue(12).Bytes(),
				},
				UnicityTreeCertificate: nil,
				UnicitySeal: &certificates.UnicitySeal{
					RootChainRoundNumber: 1,
					PreviousHash:         nil,
					Hash:                 nil,
				},
			},
		},
	}
)

func TestPartition_StartNewRoundCallsRInit(t *testing.T) {
	s := &mockTxSystem{}
	p, _ := createTestPartitionWithTxSystem(t, s)
	ucr := &certificates.UnicityCertificate{
		UnicitySeal: &certificates.UnicitySeal{
			RootChainRoundNumber: 0,
			PreviousHash:         nil,
			Hash:                 nil,
		},
	}
	p.startNewRound(ucr)
	require.Equal(t, uint64(2), s.rinitCount)
}

func TestNewPartition_NilInputParameters(t *testing.T) {
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
			ErrUnicityCertificateValidatorIsNil,
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
	p, eventBus := createPartitionWithDefaultTxSystem(t)
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
	p, eventBus := createPartitionWithDefaultTxSystem(t)
	pc10, err := eventBus.Subscribe(TopicPC1O, 10)
	require.NoError(t, err)

	p1, err := eventBus.Subscribe(TopicP1, 10)
	require.NoError(t, err)

	err = eventBus.Submit(TopicPartitionTransaction, TransactionEvent{Transaction: testtransaction.RandomBillTransfer()})
	require.NoError(t, err)

	require.Eventually(t, func() bool { <-pc10; return true }, test.WaitDuration, test.WaitTick)
	var p1Event P1Event
	require.Eventually(t, func() bool { e := <-p1; p1Event = e.(P1Event); return true }, test.WaitDuration, test.WaitTick)
	err = eventBus.Submit(TopicPartitionUnicityCertificate, UnicityCertificateEvent{
		Certificate: &certificates.UnicityCertificate{
			InputRecord:            p1Event.inputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{},
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: 2,
				PreviousHash:         nil,
				Hash:                 nil,
			},
		}},
	)

	require.NoError(t, err)
	require.Eventually(t, func() bool { <-p1; return true }, test.WaitDuration, test.WaitTick)
	height, _ := p.blockStore.Height()
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

	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	require.NoError(t, err)

	conf.PersistentPeers = []*network.PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()[0]),
		PublicKey: pubKeyBytes,
	}}
	return peer
}

func createTestPartitionWithTxSystem(t *testing.T, system TransactionSystem) (*Partition, *eventbus.EventBus) {
	selector, err := NewLeaderSelector(createPeer(t))
	require.NoError(t, err)
	ctx := context.Background()
	bus := eventbus.New()
	p, err := New(ctx, system, bus, selector, certificateValidator, txValidator, testConf)
	require.NoError(t, err)
	require.Equal(t, idle, p.status)
	t.Cleanup(p.Close)
	return p, bus
}

func createPartitionWithDefaultTxSystem(t *testing.T) (*Partition, *eventbus.EventBus) {
	return createTestPartitionWithTxSystem(t, txSystem)
}

func (m *mockTxSystem) RInit() {
	m.rinitCount++
}
func (m *mockTxSystem) Revert() {
}

func (m *mockTxSystem) RCompl() ([]byte, state.SummaryValue) {
	m.rComplCount++

	return make([]byte, 32), Uint64SummaryValue(m.rComplCount)
}

func (m *mockTxSystem) Execute(tx *transaction.Transaction) error {
	m.executeCount++
	return nil
}

func (t Uint64SummaryValue) AddToHasher(hasher hash.Hash) {
	hasher.Write(util.Uint64ToBytes(uint64(t)))
}

func (c *CertificateValidator) Validate(_ *certificates.UnicityCertificate) error {
	return nil
}

func (tv *TransactionValidator) Validate(_ *transaction.Transaction) error {
	return nil
}

func (t Uint64SummaryValue) Concatenate(left, right state.SummaryValue) state.SummaryValue {
	var s, l, r uint64
	s = uint64(t)
	lSum, ok := left.(Uint64SummaryValue)
	if ok {
		l = uint64(lSum)
	}
	rSum, ok := right.(Uint64SummaryValue)
	if ok {
		r = uint64(rSum)
	}
	return Uint64SummaryValue(s + l + r)
}

func (t Uint64SummaryValue) Bytes() []byte {
	return util.Uint64ToBytes(uint64(t))
}
