package partition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/network/protocol/replication"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

const (
	initializing status = iota
	normal
	recovering
)

func (s status) String() string {
	switch s {
	case initializing:
		return "initializing"
	case normal:
		return "normal"
	case recovering:
		return "recovering"
	default:
		return fmt.Sprintf("status(%d)", int(s))
	}
}

// Key 0 is used for proposal, that way it is still possible to reverse iterate the DB
// and use 4 byte key, make it incompatible with block number
const proposalKey = uint32(0)
const ledgerReplicationTimeout = 1500 * time.Millisecond

var ErrNodeDoesNotHaveLatestBlock = errors.New("recovery needed, node does not have the latest block")

type (
	// ValidatorNetwork provides an interface for sending and receiving validator network messages.
	ValidatorNetwork interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any

		AddTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		ForwardTransactions(ctx context.Context, receiver peer.ID)
		ProcessTransactions(ctx context.Context, txProcessor network.TxProcessor)
	}

	Observability interface {
		TracerProvider() trace.TracerProvider
		Tracer(name string, options ...trace.TracerOption) trace.Tracer
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		PrometheusRegisterer() prometheus.Registerer
		Logger() *slog.Logger
	}

	// Node represents a member in the partition and implements an instance of a specific TransactionSystem. Partition
	// is a distributed system, it consists of either a set of shards, or one or more partition nodes.
	Node struct {
		status            atomic.Value
		configuration     *configuration
		transactionSystem txsystem.TransactionSystem
		// First UC for this node. The node is guaranteed to have blocks starting at fuc+1.
		fuc *types.UnicityCertificate
		// Latest UC this node has seen. Can be ahead of the committed UC during recovery.
		luc                         atomic.Pointer[types.UnicityCertificate]
		proposedTransactions        []*types.TransactionRecord
		sumOfEarnedFees             uint64
		pendingBlockProposal        *pendingBlockProposal
		leaderSelector              LeaderSelector
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  keyvaluedb.KeyValueDB
		proofIndexer                *ProofIndexer
		stopTxProcessor             atomic.Value
		t1event                     chan struct{}
		peer                        *network.Peer
		rootNodes                   peer.IDSlice
		network                     ValidatorNetwork
		eventCh                     chan event.Event
		lastLedgerReqTime           time.Time
		eventHandler                event.Handler
		recoveryLastProp            *blockproposal.BlockProposal
		log                         *slog.Logger
		tracer                      trace.Tracer

		execTxCnt  metric.Int64Counter
		execTxDur  metric.Float64Histogram
		leaderCnt  metric.Int64Counter
		blockSize  metric.Int64Counter
		execMsgCnt metric.Int64Counter
		execMsgDur metric.Float64Histogram
	}

	pendingBlockProposal struct {
		RoundNumber     uint64
		ProposerNodeId  string
		PrevHash        []byte
		StateHash       []byte
		StateSummary    []byte
		Transactions    []*types.TransactionRecord
		SumOfEarnedFees uint64
	}

	status int
)

/*
NewNode creates a new instance of the partition node. All parameters expect the nodeOptions are required.
Functions implementing the NodeOption interface can be used to override default configuration values.

The following restrictions apply to the inputs:
  - the network peer and signer must use the same keys that were used to generate node genesis file;
  - the state of the transaction system must be equal to the state that was used to generate genesis file.
*/
func NewNode(
	ctx context.Context,
	peerConf *network.PeerConfiguration,
	signer crypto.Signer, // used to sign block proposals and block certification requests
	txSystem txsystem.TransactionSystem, // used transaction system
	genesis *genesis.PartitionGenesis, // partition genesis file, created by root chain.
	network ValidatorNetwork, // network layer of the validator node
	observe Observability,
	nodeOptions ...NodeOption, // additional optional configuration parameters
) (*Node, error) {
	tracer := observe.Tracer("partition.node", trace.WithInstrumentationAttributes(observability.PeerID(observability.NodeIDKey, peerConf.ID)))
	ctx, span := tracer.Start(ctx, "partition.NewNode")
	defer span.End()

	// load and validate node configuration
	conf, err := loadAndValidateConfiguration(signer, genesis, txSystem, observe.Logger(), nodeOptions...)
	if err != nil {
		return nil, fmt.Errorf("invalid node configuration: %w", err)
	}
	rn, err := conf.getRootNodes()
	if err != nil {
		return nil, fmt.Errorf("invalid configuration, root nodes: %w", err)
	}
	n := &Node{
		configuration:               conf,
		transactionSystem:           txSystem,
		leaderSelector:              conf.leaderSelector,
		txValidator:                 conf.txValidator,
		unicityCertificateValidator: conf.unicityCertificateValidator,
		blockProposalValidator:      conf.blockProposalValidator,
		blockStore:                  conf.blockStore,
		proofIndexer:                NewProofIndexer(conf.hashAlgorithm, conf.proofIndexConfig.store, conf.proofIndexConfig.historyLen, observe.Logger()),
		t1event:                     make(chan struct{}), // do not buffer!
		eventHandler:                conf.eventHandler,
		rootNodes:                   rn,
		network:                     network,
		lastLedgerReqTime:           time.Time{},
		log:                         observe.Logger(),
		tracer:                      tracer,
	}
	n.stopTxProcessor.Store(func() { /* init to NOP */ })
	n.status.Store(initializing)

	if err := n.initMetrics(observe); err != nil {
		return nil, fmt.Errorf("initialize metrics: %w", err)
	}

	if n.eventHandler != nil {
		n.eventCh = make(chan event.Event, conf.eventChCapacity)
	}

	if err = n.initState(ctx); err != nil {
		return nil, fmt.Errorf("node state initialization failed: %w", err)
	}

	if err = n.initNetwork(ctx, peerConf, observe); err != nil {
		return nil, fmt.Errorf("node network initialization failed: %w", err)
	}

	return n, nil
}

func (n *Node) initMetrics(observe Observability) (err error) {
	m := observe.Meter("partition.node")

	_, err = m.Int64ObservableCounter("round", metric.WithDescription("current round"),
		metric.WithInt64Callback(func(ctx context.Context, io metric.Int64Observer) error {
			io.Observe(int64(n.currentRoundNumber()))
			return nil
		}))
	if err != nil {
		return fmt.Errorf("creating counter for round number: %w", err)
	}

	n.leaderCnt, err = m.Int64Counter("round.leader", metric.WithDescription("Number of times node has been round leader"))
	if err != nil {
		return fmt.Errorf("creating counter for leader count: %w", err)
	}
	n.blockSize, err = m.Int64Counter("block.size", metric.WithDescription("Number of transactions in the proposal made by the node"), metric.WithUnit("{transaction}"))
	if err != nil {
		return fmt.Errorf("creating counter for block size: %w", err)
	}

	n.execTxCnt, err = m.Int64Counter("exec.tx.count", metric.WithDescription("Number of transactions processed by the node"), metric.WithUnit("{transaction}"))
	if err != nil {
		return fmt.Errorf("creating counter for processed tx: %w", err)
	}
	n.execTxDur, err = m.Float64Histogram("exec.tx.time",
		metric.WithDescription("How long it took to process tx (validate and execute)"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(200e-6, 400e-6, 800e-6, 0.0016, 0.003, 0.006, 0.015, 0.03))
	if err != nil {
		return fmt.Errorf("creating histogram for processed tx: %w", err)
	}

	n.execMsgCnt, err = m.Int64Counter("exec.msg.count", metric.WithDescription("Number of messages processed by the node"))
	if err != nil {
		return fmt.Errorf("creating counter for processed messages: %w", err)
	}
	n.execMsgDur, err = m.Float64Histogram("exec.msg.time",
		metric.WithDescription("How long it took to process AB network message"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(100e-6, 200e-6, 400e-6, 800e-6, 0.0016, 0.01, 0.05),
	)
	if err != nil {
		return fmt.Errorf("creating histogram for processed messages: %w", err)
	}

	return nil
}

func (n *Node) Run(ctx context.Context) error {
	// query latest UC from root
	n.sendHandshake(ctx)

	g, ctx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if n.eventHandler == nil {
			return nil // do not cancel the group!
		}
		return n.eventHandlerLoop(ctx)
	})

	// start proof indexer
	g.Go(func() error {
		return n.proofIndexer.loop(ctx)
	})

	g.Go(func() error {
		err := n.loop(ctx)
		n.log.DebugContext(ctx, "node main loop exit", logger.Error(err))
		return err
	})

	return g.Wait()
}

func (n *Node) GetPeer() *network.Peer {
	return n.peer
}

func (n *Node) initState(ctx context.Context) (err error) {
	ctx, span := n.tracer.Start(ctx, "node.initState")
	defer span.End()

	n.fuc = n.committedUC()
	luc := n.fuc

	// Apply transactions from blocks that build on the loaded
	// state. Never look further back from this starting point.
	dbIt := n.blockStore.Find(util.Uint64ToBytes(luc.GetRoundNumber() + 1))
	defer func() { err = errors.Join(err, dbIt.Close()) }()

	for ; dbIt.Valid(); dbIt.Next() {
		var bl types.Block
		roundNo := util.BytesToUint64(dbIt.Key())
		if err = dbIt.Value(&bl); err != nil {
			return fmt.Errorf("failed to read block %v from db, %w", roundNo, err)
		}
		if !bytes.Equal(luc.InputRecord.BlockHash, bl.Header.PreviousBlockHash) {
			return fmt.Errorf("invalid blockchain (previous block %v hash='%X', current block %v backlink='%X')",
				luc.GetRoundNumber(), luc.InputRecord.BlockHash, bl.GetRoundNumber(), bl.Header.PreviousBlockHash)
		}
		var state txsystem.StateSummary
		var sumOfEarnedFees uint64
		state, sumOfEarnedFees, err = n.applyBlockTransactions(ctx, bl.GetRoundNumber(), bl.Transactions)
		if err != nil {
			return fmt.Errorf("block %v apply transactions failed, %w", roundNo, err)
		}
		if err = verifyTxSystemState(state, sumOfEarnedFees, bl.UnicityCertificate.InputRecord); err != nil {
			return fmt.Errorf("block %v, state mismatch, %w", roundNo, err)
		}
		// commit changes
		if err = n.transactionSystem.Commit(bl.UnicityCertificate); err != nil {
			return fmt.Errorf("unable to commit block %v: %w", roundNo, err)
		}
		// node must have exited before block was indexed
		if n.proofIndexer.latestIndexedBlockNumber() < bl.GetRoundNumber() {
			n.proofIndexer.Handle(ctx, &bl, n.transactionSystem.StateStorage())
		}

		luc = bl.UnicityCertificate
	}

	n.log.InfoContext(ctx, fmt.Sprintf("State initialized from persistent store up to round %d", luc.GetRoundNumber()))
	n.luc.Store(luc)
	n.restoreBlockProposal(ctx)

	return err
}

func (n *Node) initNetwork(ctx context.Context, peerConf *network.PeerConfiguration, observe Observability) (err error) {
	ctx, span := n.tracer.Start(ctx, "node.initNetwork")
	defer span.End()

	n.peer, err = network.NewPeer(ctx, peerConf, n.log, observe.PrometheusRegisterer())
	if err != nil {
		return err
	}
	// add bootstrap address to the peer store
	if peerConf.BootstrapPeers != nil {
		for _, info := range peerConf.BootstrapPeers {
			for _, addr := range info.Addrs {
				n.peer.Network().Peerstore().AddAddr(info.ID, addr, peerstore.PermanentAddrTTL)
			}
		}
	}

	if n.network != nil {
		return nil
	}

	opts := network.DefaultValidatorNetworkOptions
	opts.TxBufferHashAlgorithm = n.configuration.hashAlgorithm

	n.network, err = network.NewLibP2PValidatorNetwork(n.peer, opts, observe)
	if err != nil {
		return err
	}

	return nil
}

func (n *Node) committedUC() *types.UnicityCertificate {
	return n.transactionSystem.CommittedUC()
}

func (n *Node) currentRoundNumber() uint64 {
	return n.luc.Load().GetRoundNumber() + 1
}

func (n *Node) sendHandshake(ctx context.Context) {
	n.log.DebugContext(ctx, "sending handshake to root chain")
	// select two random root nodes
	rootIDs, err := randomNodeSelector(n.rootNodes, defaultHandshakeNodes)
	// error should only happen in case the root nodes are not initialized
	if err != nil {
		n.log.WarnContext(ctx, "selecting root nodes for handshake", logger.Error(err))
		return
	}
	if err = n.network.Send(ctx,
		handshake.Handshake{
			SystemIdentifier: n.configuration.GetSystemIdentifier(),
			NodeIdentifier:   n.peer.ID().String(),
		},
		rootIDs...); err != nil {
		n.log.WarnContext(ctx, "error sending handshake", logger.Error(err))
	}
}

func verifyTxSystemState(state txsystem.StateSummary, sumOfEarnedFees uint64, ucIR *types.InputRecord) error {
	if ucIR == nil {
		return errors.New("unicity certificate input record is nil")
	}
	if !bytes.Equal(ucIR.Hash, state.Root()) {
		return fmt.Errorf("tx system state does not match unicity certificate, expected '%X', got '%X'", ucIR.Hash, state.Root())
	} else if !bytes.Equal(ucIR.SummaryValue, state.Summary()) {
		return fmt.Errorf("tx system summary value %X not equal to unicity certificate value %X", ucIR.SummaryValue, state.Summary())
	} else if ucIR.SumOfEarnedFees != sumOfEarnedFees {
		return fmt.Errorf("tx system sum of earned fees %d not equal to unicity certificate value %d", ucIR.SumOfEarnedFees, sumOfEarnedFees)
	}
	return nil
}

func (n *Node) applyBlockTransactions(ctx context.Context, round uint64, txs []*types.TransactionRecord) (txsystem.StateSummary, uint64, error) {
	ctx, span := n.tracer.Start(ctx, "node.applyBlockTransactions", trace.WithAttributes(attribute.Int64("round", int64(round))))
	defer span.End()

	var sumOfEarnedFees uint64
	if err := n.transactionSystem.BeginBlock(round); err != nil {
		return nil, 0, err
	}
	for _, tx := range txs {
		sm, err := n.validateAndExecuteTx(ctx, tx.TransactionOrder, round)
		if err != nil {
			n.log.WarnContext(ctx, "processing transaction", logger.Error(err), logger.UnitID(tx.TransactionOrder.UnitID()))
			return nil, 0, fmt.Errorf("processing transaction '%v': %w", tx.TransactionOrder.UnitID(), err)
		}
		sumOfEarnedFees += sm.ActualFee
	}
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return nil, 0, err
	}
	return state, sumOfEarnedFees, nil
}

func (n *Node) restoreBlockProposal(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.restoreBlockProposal")
	defer span.End()

	pr := &pendingBlockProposal{}
	found, err := n.blockStore.Read(util.Uint32ToBytes(proposalKey), pr)
	if err != nil {
		n.log.ErrorContext(ctx, "Error fetching block proposal", logger.Error(err))
		return
	}
	if !found {
		n.log.DebugContext(ctx, "No pending block proposal stored")
		return
	}
	// make sure proposal extends the committed state
	if !bytes.Equal(n.committedUC().InputRecord.Hash, pr.PrevHash) {
		n.log.DebugContext(ctx, "Stored block proposal does not extend previous state, stale proposal")
		return
	}
	// apply stored proposal to current state
	n.log.DebugContext(ctx, "Stored block proposal extends the previous state")
	roundNo := n.committedUC().GetRoundNumber() + 1
	state, sumOfEarnedFees, err := n.applyBlockTransactions(ctx, roundNo, pr.Transactions)
	if err != nil {
		n.log.WarnContext(ctx, "Block proposal recovery failed", logger.Error(err))
		n.revertState()
		return
	}
	if !bytes.Equal(pr.StateHash, state.Root()) {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, state hash mismatch (expected '%X', actual '%X')", pr.StateHash, state.Root()))
		n.revertState()
		return
	}
	if !bytes.Equal(pr.StateSummary, state.Summary()) {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, state summary mismatch (expected '%X', actual '%X')", pr.StateSummary, state.Summary()))
		n.revertState()
		return
	}
	if pr.SumOfEarnedFees != sumOfEarnedFees {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, sum of earned fees mismatch (expected '%d', actual '%d')", pr.SumOfEarnedFees, sumOfEarnedFees))
		n.revertState()
		return
	}
	// wait for UC to certify the block proposal
	n.pendingBlockProposal = pr
}

/*
loop runs the main loop of validator node.
It handles messages received from other AB network nodes and coordinates
timeout and communication with rootchain.
*/
func (n *Node) loop(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastUCReceived time.Time

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m, ok := <-n.network.ReceivedChannel():
			if !ok {
				return errors.New("network received channel is closed")
			}
			n.log.Log(ctx, logger.LevelTrace, fmt.Sprintf("received %T", m), logger.Data(m))

			if err := n.handleMessage(ctx, m); err != nil {
				n.log.Warn(fmt.Sprintf("handling %T", m), logger.Error(err))
			} else if _, ok := m.(*types.UnicityCertificate); ok {
				lastUCReceived = time.Now()
			}
		case <-n.t1event:
			n.handleT1TimeoutEvent(ctx)
		case <-ticker.C:
			n.handleMonitoring(ctx, lastUCReceived)
		}
	}
}

/*
handleMessage processes message received from AB network (ie from other partition nodes or rootchain).
*/
func (n *Node) handleMessage(ctx context.Context, msg any) (rErr error) {
	msgAttr := attribute.String("msg", fmt.Sprintf("%T", msg))
	ctx, span := n.tracer.Start(ctx, "node.handleMessage", trace.WithNewRoot(), trace.WithAttributes(msgAttr, n.attrRound()), trace.WithSpanKind(trace.SpanKindServer))
	defer func(start time.Time) {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
			n.sendEvent(event.Error, rErr)
		}
		n.execMsgCnt.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(msgAttr, observability.ErrStatus(rErr))))
		n.execMsgDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(msgAttr))
		span.End()
	}(time.Now())

	switch mt := msg.(type) {
	case *types.UnicityCertificate:
		return n.handleUnicityCertificate(ctx, mt)
	case *blockproposal.BlockProposal:
		return n.handleBlockProposal(ctx, mt)
	case *replication.LedgerReplicationRequest:
		return n.handleLedgerReplicationRequest(ctx, mt)
	case *replication.LedgerReplicationResponse:
		return n.handleLedgerReplicationResponse(ctx, mt)
	default:
		return fmt.Errorf("unknown message: %T", mt)
	}
}

func (n *Node) sendEvent(eventType event.Type, content any) {
	if n.eventHandler != nil {
		n.eventCh <- event.Event{
			EventType: eventType,
			Content:   content,
		}
	}
}

// eventHandlerLoop forwards events produced by a node to the configured eventHandler.
func (n *Node) eventHandlerLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case e := <-n.eventCh:
			n.eventHandler(&e)
		}
	}
}

func statusCodeOfTxError(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, ErrTxTimeout):
		return "tx.timeout"
	case errors.Is(err, errInvalidSystemIdentifier):
		return "invalid.sysid"
	default:
		return "err"
	}
}

func (n *Node) process(ctx context.Context, tx *types.TransactionOrder) (rErr error) {
	sm, err := n.validateAndExecuteTx(ctx, tx, n.committedUC().GetRoundNumber()+1)
	if err != nil {
		n.sendEvent(event.TransactionFailed, tx)
		return fmt.Errorf("executing transaction %X: %w", tx.Hash(n.configuration.hashAlgorithm), err)
	}
	n.proposedTransactions = append(n.proposedTransactions, &types.TransactionRecord{TransactionOrder: tx, ServerMetadata: sm})
	n.sumOfEarnedFees += sm.GetActualFee()
	n.sendEvent(event.TransactionProcessed, tx)
	n.log.DebugContext(ctx, fmt.Sprintf("transaction processed, proposal size: %d", len(n.proposedTransactions)), logger.UnitID(tx.UnitID()))
	return nil
}

func (n *Node) validateAndExecuteTx(ctx context.Context, tx *types.TransactionOrder, round uint64) (_ *types.ServerMetadata, rErr error) {
	defer func(start time.Time) {
		txtattr := attribute.String("tx", tx.PayloadType())
		n.execTxCnt.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(txtattr, attribute.String("status", statusCodeOfTxError(rErr)))))
		n.execTxDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributeSet(attribute.NewSet(txtattr)))
	}(time.Now())

	if err := n.txValidator.Validate(tx, round); err != nil {
		return nil, fmt.Errorf("invalid transaction: %w", err)
	}
	sm, err := n.transactionSystem.Execute(tx)
	if err != nil {
		return nil, fmt.Errorf("executing transaction in tx system: %w", err)
	}
	return sm, nil
}

// handleBlockProposal processes a block proposals. Performs the following steps:
//  1. Block proposal as a whole is validated:
//     * It must have valid signature, correct transaction system ID, valid UC;
//     * the UC must be not older than the latest known by current node;
//     * Sender must be the leader for the round started by included UC.
//  2. If included UC is newer than latest UC then the new UC is processed; this rolls back possible pending change in
//     the transaction system. If new UC is ‘repeat UC’ then update is reasonably fast; if recovery is necessary then
//     likely it takes some time and there is no reason to finish the processing of current proposal.
//  3. If the transaction system root is not equal to one extended by the processed proposal then processing is aborted.
//  4. All transaction orders in proposal are validated; on encountering an invalid transaction order the processing is
//     aborted.
//  5. Transaction orders are executed by applying them to the transaction system.
//  6. Pending unicity certificate request data structure is created and persisted.
//  7. Certificate Request query is assembled and sent to the Root Chain.
func (n *Node) handleBlockProposal(ctx context.Context, prop *blockproposal.BlockProposal) error {
	if n.status.Load() == recovering {
		// but remember last block proposal received
		n.recoveryLastProp = prop
		return fmt.Errorf("node is in recovery status")
	}
	if prop == nil {
		return blockproposal.ErrBlockProposalIsNil
	}
	nodeSignatureVerifier, err := n.configuration.GetSigningPublicKey(prop.NodeIdentifier)
	if err != nil {
		return fmt.Errorf("unknown node id, %w", err)
	}
	if err = n.blockProposalValidator.Validate(prop, nodeSignatureVerifier); err != nil {
		return fmt.Errorf("block proposal validation failed, %w", err)
	}
	n.log.DebugContext(ctx, fmt.Sprintf("Handling block proposal, its UC IR Hash %X, Block hash %X", prop.UnicityCertificate.InputRecord.Hash, prop.UnicityCertificate.InputRecord.BlockHash))
	// verify proposal unicity certificate, must not be older than latest seen received by the node
	uc := prop.UnicityCertificate
	lucRoundNumber := n.luc.Load().GetRoundNumber()
	// UC must be newer than the last one seen
	if uc.GetRoundNumber() < lucRoundNumber {
		return fmt.Errorf("outdated block proposal for round %v, LUC round %v", uc.GetRoundNumber(), lucRoundNumber)
	}
	expectedLeader := n.leaderSelector.LeaderFunc(uc, n.peer.Validators())
	if expectedLeader == UnknownLeader || prop.NodeIdentifier != expectedLeader.String() {
		return fmt.Errorf("invalid node identifier. leader from UC: %v, request leader: %v", expectedLeader, prop.NodeIdentifier)
	}
	if uc.GetRoundNumber() > lucRoundNumber {
		// either the other node received it faster from root or there must be some issue with root communication?
		n.log.DebugContext(ctx, fmt.Sprintf("Received newer UC round nr %d via block proposal, LUC round %d", uc.GetRoundNumber(), lucRoundNumber))
		if err = n.handleUnicityCertificate(ctx, uc); err != nil {
			return fmt.Errorf("block proposal unicity certificate handling failed: %w", err)
		}
	}
	prevHash := uc.InputRecord.Hash
	txState, err := n.transactionSystem.StateSummary()
	if err != nil {
		return fmt.Errorf("tx system state error, %w", err)
	}
	// check previous state matches before processing transactions
	if !bytes.Equal(prevHash, txState.Root()) {
		return fmt.Errorf("tx system start state mismatch error, expected: %X, got: %X", txState.Root(), prevHash)
	}
	if err := n.transactionSystem.BeginBlock(n.currentRoundNumber()); err != nil {
		return fmt.Errorf("tx system BeginBlock error, %w", err)
	}
	for _, tx := range prop.Transactions {
		if err = n.process(ctx, tx.TransactionOrder); err != nil {
			return fmt.Errorf("processing transaction %X: %w", tx.Hash(n.configuration.hashAlgorithm), err)
		}
	}
	if err = n.sendCertificationRequest(ctx, prop.NodeIdentifier); err != nil {
		return fmt.Errorf("certification request send failed, %w", err)
	}
	return nil
}

func (n *Node) updateLUC(uc *types.UnicityCertificate) error {
	luc := n.luc.Load()
	// Unicity Certificate GetRoundNumber function handles nil pointer and returns 0 if either UC is nil or
	// input record is nil. More importantly we should not get here if UC is nil
	if uc.GetRoundNumber() < luc.GetRoundNumber() {
		return fmt.Errorf("received stale unicity certificate, current uc round %d, received uc round %d",
			luc.GetRoundNumber(), uc.GetRoundNumber())
	}
	if n.luc.CompareAndSwap(luc, uc) {
		n.log.Debug("updated LUC", logger.Round(uc.GetRoundNumber()))
		n.sendEvent(event.LatestUnicityCertificateUpdated, uc)
	}
	return nil
}

func (n *Node) startNewRound(ctx context.Context, uc *types.UnicityCertificate) error {
	ctx, span := n.tracer.Start(ctx, "node.startNewRound")
	defer span.End()

	if err := n.updateLUC(uc); err != nil {
		return fmt.Errorf("failed to update unicity certificate: %w", err)
	}
	if n.status.Load() == recovering {
		n.log.InfoContext(ctx, "node is recovered", logger.Round(uc.InputRecord.RoundNumber))
		n.sendEvent(event.RecoveryFinished, uc.InputRecord.RoundNumber)
	}
	n.status.Store(normal)
	newRoundNr := uc.InputRecord.RoundNumber + 1
	n.resetProposal()
	n.sumOfEarnedFees = 0
	// not a fatal issue, but log anyway
	if err := n.blockStore.Delete(util.Uint32ToBytes(proposalKey)); err != nil {
		n.log.DebugContext(ctx, "DB proposal delete failed", logger.Error(err))
	}
	n.leaderSelector.UpdateLeader(uc, n.peer.Validators())
	if n.leaderSelector.IsLeader(n.peer.ID()) {
		// followers will start the block once proposal is received
		if err := n.transactionSystem.BeginBlock(newRoundNr); err != nil {
			return fmt.Errorf("starting new block for round %d: %w", newRoundNr, err)
		}
		n.leaderCnt.Add(ctx, 1)
	}
	n.startHandleOrForwardTransactions(ctx)
	n.sendEvent(event.NewRoundStarted, newRoundNr)
	return nil
}

func (n *Node) startRecovery(ctx context.Context, uc *types.UnicityCertificate) {
	ctx, span := n.tracer.Start(ctx, "node.startRecovery")
	defer span.End()
	// always update last UC seen, this is needed to evaluate if node has recovered and is up-to-date
	if err := n.updateLUC(uc); err != nil {
		n.log.WarnContext(ctx, "Start recovery unicity certificate update failed", logger.Error(err))
		return
	}
	luc := n.luc.Load()
	if n.status.Load() == recovering {
		// already recovering, but if uc is newer than luc, let's update luc
		n.log.DebugContext(ctx, fmt.Sprintf("Recovery already in progress, recovering to %v", luc.GetRoundNumber()))
		return
	}
	// starting recovery
	n.status.Store(recovering)
	n.revertState()
	n.resetProposal()
	n.stopForwardingOrHandlingTransactions()

	fromBlockNr := n.committedUC().GetRoundNumber() + 1
	n.log.DebugContext(ctx, fmt.Sprintf("Entering recovery state, recover node from %d up to round %d", fromBlockNr, luc.GetRoundNumber()))
	n.sendEvent(event.RecoveryStarted, fromBlockNr)
	n.sendLedgerReplicationRequest(ctx, fromBlockNr)
}

// handleUnicityCertificate processes the Unicity Certificate and finalizes a block. Performs the following steps:
//  1. Given UC is validated cryptographically -> checked before this method is called by unicityCertificateValidator
//  2. Given UC has correct system identifier -> checked before this method is called by unicityCertificateValidator
//  3. TODO: sanity check timestamp
//  4. Given UC is checked for equivocation (for more details see certificates.CheckNonEquivocatingCertificates)
//  5. On unexpected case where there is no pending block proposal, recovery is initiated, unless the state is already
//     up-to-date with the given UC.
//  6. Alternatively, if UC certifies the pending block proposal then block is finalized.
//  7. Alternatively, if UC certifies repeat IR (‘repeat UC’) then
//     state is rolled back to previous state.
//  8. Alternatively, recovery is initiated, after rollback. Note that recovery may end up with
//     newer last known UC than the one being processed.
//  9. New round is started.
//
// See algorithm 5 "Processing a received Unicity Certificate" in Yellowpaper for more details
func (n *Node) handleUnicityCertificate(ctx context.Context, uc *types.UnicityCertificate) error {
	if uc == nil {
		return fmt.Errorf("unicity certificate is nil")
	}
	// UC is validated cryptographically
	if err := n.unicityCertificateValidator.Validate(uc); err != nil {
		n.sendEvent(event.Error, err)
		return fmt.Errorf("certificate invalid, %w", err)
	}
	// validation must make sure all mandatory fields are present and UC is cryptographically sound
	// from this point fields can be logged, that must not be nil can be logged
	luc := n.luc.Load()

	printUC := func(uc *types.UnicityCertificate) string {
		return fmt.Sprintf("H:\t%X\nH':\t%X\nHb:\t%X\nfees:%d, round:%d, root round:%d", uc.InputRecord.Hash, uc.InputRecord.PreviousHash, uc.InputRecord.BlockHash, uc.InputRecord.SumOfEarnedFees, uc.GetRoundNumber(), uc.GetRootRoundNumber())
	}
	n.log.DebugContext(ctx, fmt.Sprintf("Received UC:\n%s", printUC(uc)))
	n.log.DebugContext(ctx, fmt.Sprintf("LUC:\n%s", printUC(luc)))

	// check for equivocation
	if err := types.CheckNonEquivocatingCertificates(luc, uc); err != nil {
		// this is not normal, log all info
		n.log.WarnContext(ctx, fmt.Sprintf("equivocating UC for round %d", uc.InputRecord.RoundNumber), logger.Error(err), logger.Data(uc))
		n.log.WarnContext(ctx, "LUC", logger.Data(luc))
		return fmt.Errorf("equivocating certificate: %w", err)
	}

	if uc.GetRootRoundNumber() == luc.GetRootRoundNumber() {
		n.log.DebugContext(ctx, "duplicate UC (same root round)", logger.Round(uc.GetRootRoundNumber()))
		if n.status.Load() == initializing {
			return n.startNewRound(ctx, uc)
		}
		return nil
	}

	if n.status.Load() == recovering {
		n.log.DebugContext(ctx, "Recovery already in progress, updating LUC")
		if err := n.updateLUC(uc); err != nil {
			return fmt.Errorf("updating LUC: %w", err)
		}
		return nil
	}

	if uc.IsRepeat(luc) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		n.log.WarnContext(ctx, fmt.Sprintf("Reverting state tree on repeat certificate. UC IR hash: %X; %s", uc.InputRecord.Hash, n.pendingBlockProposal.pretty()))
		n.revertState()
		return n.startNewRound(ctx, uc)
	}

	lastCommittedRoundNumber := n.committedUC().GetRoundNumber()
	if uc.GetRoundNumber() != lastCommittedRoundNumber+1 {
		// do not allow gaps between blocks, even if state hash does not change
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, missing blocks. UC round number: %d, current round number: %d", uc.GetRoundNumber(), lastCommittedRoundNumber+1))
		n.startRecovery(ctx, uc)
		return ErrNodeDoesNotHaveLatestBlock
	}

	// If there is no pending block proposal i.e. no certification request has been sent by the node
	// - leader was down and did not make a block proposal?
	// - node did not receive a block proposal because it was down, it was not sent or there were network issues
	// Note, if for any reason the node misses the proposal and other validators finalize _one_ empty block,
	// this node will start a new round (the one that has been already finalized).
	// Eventually it will start the recovery and catch up.
	if n.pendingBlockProposal == nil {
		// Start recovery unless the state is already up-to-date with UC.
		state, err := n.transactionSystem.StateSummary()
		if err != nil {
			n.startRecovery(ctx, uc)
			return fmt.Errorf("recovery needed, failed to get tx system state: %w", err)
		}
		// if state hash does not match - start recovery
		if !bytes.Equal(uc.InputRecord.Hash, state.Root()) {
			n.startRecovery(ctx, uc)
			return ErrNodeDoesNotHaveLatestBlock
		}
		n.log.DebugContext(ctx, "No pending block proposal, UC IR hash is equal to State hash, so are block hashes")
		return n.startNewRound(ctx, uc)
	}
	// Check pending block proposal
	bl, blockHash, err := n.proposalHash(n.pendingBlockProposal, uc)
	n.log.DebugContext(ctx, fmt.Sprintf("%s\nHb:\t%X", n.pendingBlockProposal.pretty(), blockHash))
	if err != nil {
		n.startRecovery(ctx, uc)
		return fmt.Errorf("recovery needed, block proposal hash calculation failed: %w", err)
	}

	if !bytes.Equal(uc.GetStateHash(), n.pendingBlockProposal.StateHash) {
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, proposal's state hash is different (UC: '%X', actual '%X')", uc.GetStateHash(), n.pendingBlockProposal.StateHash))
	} else if !bytes.Equal(uc.InputRecord.SummaryValue, n.pendingBlockProposal.StateSummary) {
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, proposal's state summary is different (UC: '%X', actual '%X')", uc.InputRecord.SummaryValue, n.pendingBlockProposal.StateSummary))
	} else if uc.InputRecord.SumOfEarnedFees != n.pendingBlockProposal.SumOfEarnedFees {
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, proposal's sum of earned fees is different (UC: %d, actual %d)", uc.InputRecord.SumOfEarnedFees, n.pendingBlockProposal.SumOfEarnedFees))
	} else {
		// UC certifies pending block proposal
		if err = n.finalizeBlock(ctx, bl); err != nil {
			n.startRecovery(ctx, uc)
			return fmt.Errorf("block %v finalize failed: %w", bl.GetRoundNumber(), err)
		}
		return n.startNewRound(ctx, uc)
	}

	// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
	// revertState is called from startRecovery()
	n.startRecovery(ctx, uc)
	return ErrNodeDoesNotHaveLatestBlock
}

func (n *Node) revertState() {
	n.log.Warn("Reverting state")
	n.sendEvent(event.StateReverted, nil)
	n.transactionSystem.Revert()
	n.sumOfEarnedFees = 0
}

func (n *Node) proposalHash(prop *pendingBlockProposal, uc *types.UnicityCertificate) (*types.Block, []byte, error) {
	b := &types.Block{
		Header: &types.Header{
			SystemID:          n.configuration.GetSystemIdentifier(),
			ShardID:           nil,
			ProposerID:        prop.ProposerNodeId,
			PreviousBlockHash: n.committedUC().InputRecord.BlockHash,
		},
		Transactions:       prop.Transactions,
		UnicityCertificate: uc,
	}
	blockHash, err := b.Hash(n.configuration.hashAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("block hash calculation failed, %w", err)
	}
	return b, blockHash, nil
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(ctx context.Context, b *types.Block) error {
	blockNumber := b.GetRoundNumber()
	_, span := n.tracer.Start(ctx, "Node.finalizeBlock", trace.WithAttributes(attribute.Int64("block.number", int64(blockNumber))))
	defer span.End()

	roundNoInBytes := util.Uint64ToBytes(blockNumber)
	// persist the block _before_ committing to tx system
	// if write fails but the round is committed in tx system, there's no way back,
	// but if commit fails, we just remove the block from the store
	if err := n.blockStore.Write(roundNoInBytes, b); err != nil {
		return fmt.Errorf("db write failed, %w", err)
	}

	if err := n.transactionSystem.Commit(b.UnicityCertificate); err != nil {
		err = fmt.Errorf("unable to finalize block %d: %w", blockNumber, err)
		if err2 := n.blockStore.Delete(roundNoInBytes); err2 != nil {
			err = errors.Join(err, fmt.Errorf("unable to delete block %d from store: %w", blockNumber, err2))
		}
		return err
	}
	n.sendEvent(event.BlockFinalized, b)
	n.proofIndexer.Handle(ctx, b, n.transactionSystem.StateStorage())
	return nil
}

func (n *Node) handleT1TimeoutEvent(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.handleT1TimeoutEvent", trace.WithNewRoot(), trace.WithAttributes(n.attrRound()))
	defer span.End()

	n.stopForwardingOrHandlingTransactions()
	defer func() { n.leaderSelector.UpdateLeader(nil, n.peer.Validators()) }()

	if n.status.Load() == recovering {
		n.log.InfoContext(ctx, "T1 timeout: node is recovering")
		return
	}
	n.log.InfoContext(ctx, "Handling T1 timeout")
	// if node is not leader, then do not do anything
	if !n.leaderSelector.IsLeader(n.peer.ID()) {
		n.log.DebugContext(ctx, "Current node is not the leader.")
		return
	}
	n.log.DebugContext(ctx, "Current node is the leader.")
	if err := n.sendBlockProposal(ctx); err != nil {
		n.log.WarnContext(ctx, "Failed to send BlockProposal", logger.Error(err))
		return
	}
	if err := n.sendCertificationRequest(ctx, n.peer.ID().String()); err != nil {
		n.log.WarnContext(ctx, "Failed to send certification request", logger.Error(err))
	}
}

// handleMonitoring - monitors root communication, if for no UC is
// received for a long time then try and request one from root
func (n *Node) handleMonitoring(ctx context.Context, lastUCReceived time.Time) {
	ctx, span := n.tracer.Start(ctx, "node.handleMonitoring", trace.WithNewRoot(), trace.WithAttributes(n.attrRound(), attribute.String("last UC", lastUCReceived.String())))
	defer span.End()

	// check if we have not heard from root validator for T2 timeout + 1 sec
	// a new repeat UC must have been made by now (assuming root is fine) try and get it from other root nodes
	if time.Since(lastUCReceived) > n.configuration.GetT2Timeout()+time.Second {
		// query latest UC from root
		n.sendHandshake(ctx)
	}
	// handle ledger replication timeout - no response from node is received
	if n.status.Load() == recovering && time.Since(n.lastLedgerReqTime) > ledgerReplicationTimeout {
		n.log.WarnContext(ctx, "Ledger replication timeout, repeat request")
		n.sendLedgerReplicationRequest(ctx, n.committedUC().GetRoundNumber()+1)
	}
}

func (n *Node) sendLedgerReplicationResponse(ctx context.Context, msg *replication.LedgerReplicationResponse, toId string) error {
	n.log.DebugContext(ctx, fmt.Sprintf("Sending ledger replication response to %s: %s", toId, msg.Pretty()))
	recoveringNodeID, err := peer.Decode(toId)
	if err != nil {
		return fmt.Errorf("decoding peer id %q: %w", toId, err)
	}

	if err = n.network.Send(ctx, msg, recoveringNodeID); err != nil {
		return fmt.Errorf("sending replication response: %w", err)
	}
	n.sendEvent(event.ReplicationResponseSent, msg)
	return nil
}

func (n *Node) handleLedgerReplicationRequest(ctx context.Context, lr *replication.LedgerReplicationRequest) error {
	n.log.DebugContext(ctx, fmt.Sprintf("Handling ledger replication request from '%s', starting block %d", lr.NodeIdentifier, lr.BeginBlockNumber))
	if err := lr.IsValid(); err != nil {
		// for now do not respond to obviously invalid requests
		return fmt.Errorf("invalid request, %w", err)
	}
	// check if the node is known
	if _, err := n.configuration.GetSigningPublicKey(lr.NodeIdentifier); err != nil {
		return fmt.Errorf("unknown node, %w", err)
	}
	if lr.SystemIdentifier != n.configuration.GetSystemIdentifier() {
		resp := &replication.LedgerReplicationResponse{
			Status:  replication.UnknownSystemIdentifier,
			Message: fmt.Sprintf("Unknown system identifier: %s", lr.SystemIdentifier),
		}
		return n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier)
	}
	startBlock := lr.BeginBlockNumber
	// the node has been started with a later state and does not have the needed data
	if startBlock <= n.fuc.GetRoundNumber() {
		resp := &replication.LedgerReplicationResponse{
			Status:  replication.BlocksNotFound,
			Message: fmt.Sprintf("Node does not have block: %v, first block: %v", startBlock, n.fuc.GetRoundNumber()+1),
		}
		return n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier)
	}
	// the node is behind and does not have the needed data
	latestBlock := n.committedUC().GetRoundNumber()
	if latestBlock < startBlock {
		resp := &replication.LedgerReplicationResponse{
			Status:  replication.BlocksNotFound,
			Message: fmt.Sprintf("Node does not have block: %v, latest block: %v", startBlock, latestBlock),
		}
		return n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier)
	}
	n.log.DebugContext(ctx, fmt.Sprintf("Preparing replication response from block %d", startBlock))
	go func() {
		blocks := make([]*types.Block, 0)
		countTx := uint32(0)
		blockCnt := uint64(0)
		dbIt := n.blockStore.Find(util.Uint64ToBytes(startBlock))
		defer func() {
			if err := dbIt.Close(); err != nil {
				n.log.WarnContext(ctx, "closing DB iterator", logger.Error(err))
			}
		}()
		var lastFetchedBlock *types.Block
		for ; dbIt.Valid(); dbIt.Next() {
			var bl types.Block
			roundNo := util.BytesToUint64(dbIt.Key())
			if err := dbIt.Value(&bl); err != nil {
				n.log.WarnContext(ctx, fmt.Sprintf("Ledger replication reply incomplete, failed to read block %d", roundNo), logger.Error(err))
				break
			}
			lastFetchedBlock = &bl
			blocks = append(blocks, lastFetchedBlock)
			blockCnt++
			countTx += uint32(len(bl.Transactions))
			if countTx >= n.configuration.replicationConfig.maxTx ||
				blockCnt >= n.configuration.replicationConfig.maxBlocks {
				break
			}
		}
		resp := &replication.LedgerReplicationResponse{
			Status: replication.Ok,
			Blocks: blocks,
		}
		if err := n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier); err != nil {
			n.log.WarnContext(ctx, fmt.Sprintf("Problem sending ledger replication response, %s", resp.Pretty()), logger.Error(err))
		}
	}()
	return nil
}

// handleLedgerReplicationResponse handles ledger replication responses from other partition nodes.
// This method is an approximation of YellowPaper algorithm 10 "Partition Node Recovery" (synchronous algorithm)
func (n *Node) handleLedgerReplicationResponse(ctx context.Context, lr *replication.LedgerReplicationResponse) error {
	if err := lr.IsValid(); err != nil {
		return fmt.Errorf("invalid ledger replication response, %w", err)
	}
	if n.status.Load() != recovering {
		n.log.DebugContext(ctx, fmt.Sprintf("Stale Ledger Replication response, node is not recovering: %s", lr.Pretty()))
		return nil
	}
	n.log.DebugContext(ctx, fmt.Sprintf("Ledger replication response received: %s, ", lr.Pretty()))
	if lr.Status != replication.Ok {
		recoverFrom := n.committedUC().GetRoundNumber() + 1
		n.log.DebugContext(ctx, fmt.Sprintf("Resending replication request starting with round %d", recoverFrom))
		n.sendLedgerReplicationRequest(ctx, recoverFrom)
		return fmt.Errorf("received error response, status=%s, message='%s'", lr.Status.String(), lr.Message)
	}

	onError := func(latestSuccessfulRoundNumber uint64, err error) error {
		// log problems
		n.log.ErrorContext(ctx, "Recovery failed", logger.Error(err))
		// Revert any transactions that were applied
		n.revertState()
		// ask for the failed block again, what else can we do?
		n.sendLedgerReplicationRequest(ctx, latestSuccessfulRoundNumber+1)
		return err
	}

	committedUC := n.committedUC()
	latestProcessedRoundNumber := committedUC.GetRoundNumber()
	latestStateHash := committedUC.InputRecord.Hash

	var err error
	for _, b := range lr.Blocks {
		if err = b.IsValid(n.unicityCertificateValidator.Validate); err != nil {
			// sends invalid blocks, do not trust the response and try again
			err = fmt.Errorf("ledger replication response contains invalid block for round %v, %w", b.GetRoundNumber(), err)
			return onError(latestProcessedRoundNumber, err)
		}
		// it could be that we receive blocks from earlier time or later time, make sure to extend from what is missing
		recoveringRoundNo := b.GetRoundNumber()
		// skip earlier blocks
		if recoveringRoundNo <= latestProcessedRoundNumber {
			n.log.DebugContext(ctx, fmt.Sprintf("Node already has this block %v, skipping block %v", latestProcessedRoundNumber, recoveringRoundNo))
			continue
		} else if recoveringRoundNo > latestProcessedRoundNumber+1 {
			return onError(latestProcessedRoundNumber, fmt.Errorf("node is missing blocks between rounds %v and %v", latestProcessedRoundNumber, recoveringRoundNo))
		}
		n.log.DebugContext(ctx, fmt.Sprintf("Recovering block from round %d", recoveringRoundNo))
		// make sure it extends current state
		var state txsystem.StateSummary
		state, err = n.transactionSystem.StateSummary()
		if err != nil {
			return onError(latestProcessedRoundNumber, fmt.Errorf("error reading current state, %w", err))
		}
		if !bytes.Equal(b.UnicityCertificate.InputRecord.PreviousHash, state.Root()) {
			return onError(latestProcessedRoundNumber, fmt.Errorf("received block does not extend current state, state: %X, block's IR.PreviousHash: %X", state.Root(), b.UnicityCertificate.InputRecord.PreviousHash))
		}
		if !bytes.Equal(b.UnicityCertificate.InputRecord.PreviousHash, latestStateHash) {
			return onError(latestProcessedRoundNumber, fmt.Errorf("received block does not extend last unicity certificate"))
		}
		var sumOfEarnedFees uint64
		state, sumOfEarnedFees, err = n.applyBlockTransactions(ctx, latestProcessedRoundNumber+1, b.Transactions)
		if err != nil {
			return onError(latestProcessedRoundNumber, fmt.Errorf("block %v apply transactions failed, %w", recoveringRoundNo, err))
		}
		if err = verifyTxSystemState(state, sumOfEarnedFees, b.UnicityCertificate.InputRecord); err != nil {
			return onError(latestProcessedRoundNumber, fmt.Errorf("block %v, state mismatch, %w", recoveringRoundNo, err))
		}
		// update DB and last block
		if err = n.finalizeBlock(ctx, b); err != nil {
			return onError(latestProcessedRoundNumber, fmt.Errorf("block %v persist failed, %w", recoveringRoundNo, err))
		}
		latestProcessedRoundNumber = recoveringRoundNo
		latestStateHash = b.UnicityCertificate.InputRecord.Hash
	}

	// check if recovery is complete
	n.log.DebugContext(ctx, fmt.Sprintf("Checking if recovery is complete, last recovered round: %d", latestProcessedRoundNumber))
	// if the state hash is equal to luc state hash then recovery is complete
	luc := n.luc.Load()
	if !bytes.Equal(latestStateHash, luc.InputRecord.Hash) {
		n.log.DebugContext(ctx, fmt.Sprintf("Not fully recovered yet, latest recovered UC's round %d vs LUC's round %d", latestProcessedRoundNumber, luc.GetRoundNumber()))
		n.sendLedgerReplicationRequest(ctx, latestProcessedRoundNumber+1)
		return nil
	}
	// node should be recovered now, stop recovery and change state to normal
	if err := n.startNewRound(ctx, luc); err != nil {
		return err
	}
	// try to apply the last received block proposal received during recovery, it may fail if the block was finalized and
	// is in fact the last block received
	if n.recoveryLastProp != nil {
		// try to apply it to the latest state, may fail
		if err = n.handleBlockProposal(ctx, n.recoveryLastProp); err != nil {
			n.log.DebugContext(ctx, "Recovery completed, failed to apply last received block proposal(stale?)", logger.Error(err))
		}
		n.recoveryLastProp = nil
	}
	return nil
}

func (n *Node) sendLedgerReplicationRequest(ctx context.Context, startingBlockNr uint64) {
	ctx, span := n.tracer.Start(ctx, "node.sendLedgerReplicationRequest", trace.WithAttributes(attribute.Int64("starting_block", int64(startingBlockNr))))
	defer span.End()

	req := &replication.LedgerReplicationRequest{
		SystemIdentifier: n.configuration.GetSystemIdentifier(),
		NodeIdentifier:   n.peer.ID().String(),
		BeginBlockNumber: startingBlockNr,
	}
	n.log.Log(ctx, logger.LevelTrace, "sending ledger replication request", logger.Data(req))
	peers := n.peer.Validators()
	if len(peers) == 0 {
		n.log.WarnContext(ctx, "Error sending ledger replication request, no peers")
		return
	}

	// send Ledger Replication request to a first alive randomly chosen node
	for _, p := range util.ShuffleSliceCopy(peers) {
		if n.peer.ID() == p {
			continue
		}
		n.log.DebugContext(ctx, fmt.Sprintf("Sending ledger replication request to %v", p))
		// break loop on successful send, otherwise try again but different node, until all either
		// able to send or all attempts have failed
		if err := n.network.Send(ctx, req, p); err != nil {
			n.log.DebugContext(ctx, "Error sending ledger replication request", logger.Error(err))
			continue
		}
		// remember last request sent for timeout handling - if no response is received
		n.lastLedgerReqTime = time.Now()
		return
	}

	n.log.WarnContext(ctx, "failed to send ledger replication request (no peers, all peers down?)")
}

func (n *Node) sendBlockProposal(ctx context.Context) error {
	ctx, span := n.tracer.Start(ctx, "node.sendBlockProposal")
	defer span.End()

	systemIdentifier := n.configuration.GetSystemIdentifier()
	nodeId := n.peer.ID()
	prop := &blockproposal.BlockProposal{
		SystemIdentifier:   systemIdentifier,
		NodeIdentifier:     nodeId.String(),
		UnicityCertificate: n.luc.Load(),
		Transactions:       n.proposedTransactions,
	}
	n.log.Log(ctx, logger.LevelTrace, "created BlockProposal", logger.Data(prop))
	if err := prop.Sign(n.configuration.hashAlgorithm, n.configuration.signer); err != nil {
		return fmt.Errorf("block proposal sign failed, %w", err)
	}
	n.blockSize.Add(ctx, int64(len(prop.Transactions)))
	return n.network.Send(ctx, prop, n.peer.FilterValidators(nodeId)...)
}

func (n *Node) persistBlockProposal(pr *pendingBlockProposal) error {
	if err := n.blockStore.Write(util.Uint32ToBytes(proposalKey), pr); err != nil {
		return fmt.Errorf("persist error, %w", err)
	}
	return nil
}

func (n *Node) sendCertificationRequest(ctx context.Context, blockAuthor string) error {
	ctx, span := n.tracer.Start(ctx, "node.sendCertificationRequest", trace.WithAttributes(attribute.String("author", blockAuthor)))
	defer span.End()

	systemIdentifier := n.configuration.GetSystemIdentifier()
	luc := n.luc.Load()
	prevStateHash := luc.InputRecord.Hash
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return fmt.Errorf("tx system failed to end block, %w", err)
	}
	stateHash := state.Root()
	summary := state.Summary()
	pendingProposal := &pendingBlockProposal{
		ProposerNodeId:  blockAuthor,
		RoundNumber:     n.currentRoundNumber(),
		PrevHash:        prevStateHash,
		StateHash:       stateHash,
		StateSummary:    summary,
		Transactions:    n.proposedTransactions,
		SumOfEarnedFees: n.sumOfEarnedFees,
	}
	if err = n.persistBlockProposal(pendingProposal); err != nil {
		n.transactionSystem.Revert()
		return fmt.Errorf("failed to store pending block proposal: %w", err)
	}
	n.pendingBlockProposal = pendingProposal

	latestBlockHash := n.committedUC().InputRecord.BlockHash
	blockHash, err := n.hashProposedBlock(latestBlockHash, blockAuthor)
	if err != nil {
		return fmt.Errorf("calculating block hash: %w", err)
	}
	n.proposedTransactions = []*types.TransactionRecord{}
	n.sumOfEarnedFees = 0

	req := &certification.BlockCertificationRequest{
		SystemIdentifier: systemIdentifier,
		NodeIdentifier:   n.peer.ID().String(),
		InputRecord: &types.InputRecord{
			PreviousHash: pendingProposal.PrevHash,
			Hash:         pendingProposal.StateHash,
			BlockHash:    blockHash,
			SummaryValue: pendingProposal.StateSummary,
			// latest UC might have certified an empty block and has the latest round number
			RoundNumber:     pendingProposal.RoundNumber,
			SumOfEarnedFees: pendingProposal.SumOfEarnedFees,
		},
		RootRoundNumber: luc.UnicitySeal.RootChainRoundNumber,
	}
	if err = req.Sign(n.configuration.signer); err != nil {
		return fmt.Errorf("failed to sign certification req, %w", err)
	}
	n.log.InfoContext(ctx, fmt.Sprintf("Round %v sending block certification request to root chain, IR hash %X, Block Hash %X, fee sum %d",
		pendingProposal.RoundNumber, stateHash, blockHash, pendingProposal.SumOfEarnedFees))
	n.log.Log(ctx, logger.LevelTrace, "Block Certification req", logger.Data(req))
	rootIDs, err := rootNodesSelector(luc, n.rootNodes, defaultNofRootNodes)
	if err != nil {
		n.log.WarnContext(ctx, "root node selection error: %w", err)
	}
	return n.network.Send(ctx, req, rootIDs...)
}

func (n *Node) resetProposal() {
	n.proposedTransactions = []*types.TransactionRecord{}
	n.pendingBlockProposal = nil
}

func (n *Node) SubmitTx(ctx context.Context, tx *types.TransactionOrder) (txOrderHash []byte, err error) {
	if err = n.txValidator.Validate(tx, n.currentRoundNumber()); err != nil {
		return nil, err
	}

	return n.network.AddTransaction(ctx, tx)
}

func (n *Node) GetBlock(_ context.Context, blockNr uint64) (*types.Block, error) {
	// find and return closest match from db
	if blockNr <= n.fuc.GetRoundNumber() {
		return nil, fmt.Errorf("node does not have block: %v, first block: %v", blockNr, n.fuc.GetRoundNumber()+1)
	}
	var bl types.Block
	found, err := n.blockStore.Read(util.Uint64ToBytes(blockNr), &bl)
	if err != nil {
		return nil, fmt.Errorf("failed to read block from round %v from db, %w", blockNr, err)
	}
	if !found {
		return nil, nil
	}
	return &bl, nil
}

/*
LatestBlockNumber returns the latest committed round number.
It's part of the public API exposed by node.
*/
func (n *Node) LatestBlockNumber() (uint64, error) {
	if status := n.status.Load(); status != normal {
		return 0, fmt.Errorf("node not ready: %s", status)
	}

	return n.committedUC().GetRoundNumber(), nil
}

func (n *Node) GetTransactionRecord(ctx context.Context, hash []byte) (*types.TransactionRecord, *types.TxProof, error) {
	proofs := n.proofIndexer.GetDB()
	index, err := ReadTransactionIndex(proofs, hash)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to query tx index: %w", err)
	}
	b, err := n.GetBlock(ctx, index.RoundNumber)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to load block: %w", err)
	}
	proof, record, err := types.NewTxProof(b, index.TxOrderIndex, n.configuration.hashAlgorithm)
	if err != nil {
		return nil, nil, fmt.Errorf("unable to extract transaction record and execution proof from the block: %w", err)
	}
	h := record.TransactionOrder.Hash(n.configuration.hashAlgorithm)
	if !bytes.Equal(h, hash) {
		return nil, nil, errors.New("transaction index is invalid: hash mismatch")
	}
	return record, proof, nil
}

/*
GetLatestRoundNumber returns the round number of the latest seen UC.
It's part of the public API exposed by node.
*/
func (n *Node) GetLatestRoundNumber(ctx context.Context) (uint64, error) {
	_, span := n.tracer.Start(ctx, "node.GetLatestRoundNumber")
	defer span.End()

	if status := n.status.Load(); status != normal {
		return 0, fmt.Errorf("node not ready: %s", status)
	}
	return n.luc.Load().GetRoundNumber(), nil
}

func (n *Node) SerializeState(writer io.Writer) error {
	if status := n.status.Load(); status != normal {
		return fmt.Errorf("node not ready: %s", status)
	}

	return n.transactionSystem.SerializeState(writer, true)
}

func (n *Node) SystemIdentifier() types.SystemID {
	return n.configuration.GetSystemIdentifier()
}

func (n *Node) GetUnitState(unitID []byte, returnProof bool, returnData bool) (*types.UnitDataAndProof, error) {
	clonedState := n.transactionSystem.StateStorage()
	unit, err := clonedState.GetUnit(unitID, true)
	if err != nil {
		return nil, err
	}
	response := &types.UnitDataAndProof{}
	if returnData {
		res, err := cbor.Marshal(unit.Data())
		if err != nil {
			return nil, fmt.Errorf("failed to encode unit data: %w", err)
		}
		response.UnitData = &types.StateUnitData{Data: res, Bearer: unit.Bearer()}
	}
	if returnProof {
		p, err := clonedState.CreateUnitStateProof(unitID, len(unit.Logs())-1)
		if err != nil {
			return nil, err
		}
		response.Proof = p
	}
	return response, nil
}

func (n *Node) stopForwardingOrHandlingTransactions() {
	n.stopTxProcessor.Load().(func())()
}

func (n *Node) startHandleOrForwardTransactions(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.startHandleOrForwardTransactions", trace.WithAttributes(n.attrRound()))
	defer span.End()

	n.stopForwardingOrHandlingTransactions()

	leader := n.leaderSelector.GetLeader()
	if leader == UnknownLeader {
		n.log.Warn("unknown round leader", logger.Round(n.currentRoundNumber()))
		return
	}

	txCtx, txCancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(2)

	n.stopTxProcessor.Store(func() {
		txCancel()
		wg.Wait()
	})

	go func() {
		ctx, span := n.tracer.Start(txCtx, "node.processTransactions", trace.WithNewRoot(), trace.WithLinks(trace.LinkFromContext(txCtx)))
		defer func() {
			span.End()
			wg.Done()
		}()

		if leader == n.peer.ID() {
			n.network.ProcessTransactions(ctx, n.process)
		} else {
			n.network.ForwardTransactions(ctx, leader)
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case <-time.After(n.configuration.t1Timeout):
			// Rather than call handleT1TimeoutEvent directly send signal to main
			// loop - helps to avoid concurrency issues with (repeat) UC handling.
			select {
			case n.t1event <- struct{}{}:
			case <-txCtx.Done():
			}
		case <-txCtx.Done():
		}
	}()
}

func (n *Node) hashProposedBlock(prevBlockHash []byte, author string) ([]byte, error) {
	b := &types.Block{
		Header: &types.Header{
			SystemID:          n.configuration.GetSystemIdentifier(),
			ProposerID:        author,
			PreviousBlockHash: prevBlockHash,
		},
		Transactions: n.pendingBlockProposal.Transactions,
	}
	return b.Hash(n.configuration.hashAlgorithm)
}

func (n *Node) attrRound() attribute.KeyValue {
	return observability.Round(n.currentRoundNumber())
}

func (p *pendingBlockProposal) pretty() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("Pending proposal: \nH:\t%X\nH':\t%X\nround:\t%v\nfees:\t%d",
		p.StateHash, p.PrevHash, p.RoundNumber, p.SumOfEarnedFees)
}
