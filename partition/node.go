package partition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

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

var ErrNodeDoesNotHaveLatestBlock = errors.New("recovery needed, node does not have the latest block")

type (
	// ValidatorNetwork provides an interface for sending and receiving validator network messages.
	ValidatorNetwork interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any

		PublishBlock(ctx context.Context, block *types.Block) error
		AddTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		ForwardTransactions(ctx context.Context, receiverFunc network.TxReceiver)
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
		// TR corresponding to the latest UC this node has seen (as referenced by luc.TRHash).
		// Can be nil if latest UC was received with a block (recovery or block propagation protocols).
		ltr                         atomic.Pointer[certification.TechnicalRecord]
		proposedTransactions        []*types.TransactionRecord
		sumOfEarnedFees             uint64
		pendingBlockProposal        *types.Block
		leader                      Leader
		txValidator                 TxValidator
		unicityCertificateValidator UnicityCertificateValidator
		blockProposalValidator      BlockProposalValidator
		blockStore                  keyvaluedb.KeyValueDB
		proofIndexer                *ProofIndexer
		ownerIndexer                *OwnerIndexer
		stopTxProcessor             atomic.Value
		t1event                     chan struct{}
		peer                        *network.Peer
		rootNodes                   peer.IDSlice
		validatorNodes              peer.IDSlice
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

	status int
)

/*
NewNode creates a new instance of the partition node. All parameters expect the nodeOptions are required.
Functions implementing the NodeOption interface can be used to override default configuration values.

The following restrictions apply to the inputs:
  - the network peer and signer must use the same keys that were used to generate node genesis file;
*/
func NewNode(
	ctx context.Context,
	peerConf *network.PeerConfiguration,
	signer crypto.Signer, // used to sign block proposals and block certification requests
	txSystem txsystem.TransactionSystem, // used transaction system
	genesis *genesis.PartitionGenesis, // partition genesis file, created by root chain.
	trustBase types.RootTrustBase, // root trust base file
	network ValidatorNetwork, // network layer of the validator node
	observe Observability,
	nodeOptions ...NodeOption, // additional optional configuration parameters
) (*Node, error) {
	tracer := observe.Tracer("partition.node", trace.WithInstrumentationAttributes(observability.PeerID(observability.NodeIDKey, peerConf.ID)))
	ctx, span := tracer.Start(ctx, "partition.NewNode")
	defer span.End()

	// load and validate node configuration
	conf, err := loadAndValidateConfiguration(signer, genesis, trustBase, txSystem, nodeOptions...)
	if err != nil {
		return nil, fmt.Errorf("invalid node configuration: %w", err)
	}
	rn, err := conf.getRootNodes()
	if err != nil {
		return nil, fmt.Errorf("invalid configuration, root nodes: %w", err)
	}

	// load owner indexer
	if conf.ownerIndexer != nil {
		if err := conf.ownerIndexer.LoadState(txSystem.State()); err != nil {
			return nil, fmt.Errorf("failed to initialize state in proof indexer: %w", err)
		}
	}
	n := &Node{
		configuration:               conf,
		transactionSystem:           txSystem,
		txValidator:                 conf.txValidator,
		unicityCertificateValidator: conf.unicityCertificateValidator,
		blockProposalValidator:      conf.blockProposalValidator,
		blockStore:                  conf.blockStore,
		proofIndexer:                NewProofIndexer(conf.hashAlgorithm, conf.proofIndexConfig.store, conf.proofIndexConfig.historyLen, observe.Logger()),
		ownerIndexer:                conf.ownerIndexer,
		t1event:                     make(chan struct{}), // do not buffer!
		eventHandler:                conf.eventHandler,
		rootNodes:                   rn,
		validatorNodes:              peerConf.Validators,
		network:                     network,
		lastLedgerReqTime:           time.Time{},
		log:                         observe.Logger(),
		tracer:                      tracer,
	}
	n.resetProposal()
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
		metric.WithDescription("How long it took to process transaction (validate and execute)"),
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
	if n.IsValidatorNode() {
		// Validator nodes query for the latest UC from root. They retain the status
		// 'initializing' until the first UC is received. Then they'll know the leader
		// and can either start a new round or go into recovery.
		n.sendHandshake(ctx)
	} else {
		// Non-validator nodes do not need to wait for the first UC to finish initialization.
		// They can start processing new blocks and forwarding transactions as their normal
		// operation mode immediately.
		n.status.Store(normal)
		go n.network.ForwardTransactions(ctx, n.leader.Get)
	}

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

func (n *Node) initState(ctx context.Context) (err error) {
	ctx, span := n.tracer.Start(ctx, "node.initState")
	defer span.End()

	n.fuc = n.committedUC()
	n.luc.Store(n.fuc)

	// Apply transactions from blocks that build on the loaded
	// state. Never look further back from this starting point.
	dbIt := n.blockStore.Find(util.Uint64ToBytes(n.fuc.GetRoundNumber() + 1))
	defer func() { err = errors.Join(err, dbIt.Close()) }()

	for ; dbIt.Valid(); dbIt.Next() {
		var b types.Block
		roundNo := util.BytesToUint64(dbIt.Key())
		if err = dbIt.Value(&b); err != nil {
			return fmt.Errorf("failed to read block %v from db: %w", roundNo, err)
		}
		if err = n.handleBlock(ctx, &b); err != nil {
			return fmt.Errorf("failed to handle block %v: %w", roundNo, err)
		}
	}

	n.log.InfoContext(ctx, fmt.Sprintf("State initialized from persistent store up to round %d", n.committedUC().GetRoundNumber()))
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
	if n.network != nil {
		return nil
	}

	opts := network.DefaultValidatorNetworkOptions
	opts.TxBufferHashAlgorithm = n.configuration.hashAlgorithm

	n.network, err = network.NewLibP2PValidatorNetwork(ctx, n, opts, observe)
	if err != nil {
		return err
	}

	// Open a connection to the bootstrap nodes.
	// This is the only way to discover other peers, so let's do this as soon as possible.
	if err := n.peer.BootstrapConnect(ctx, n.log); err != nil {
		return err
	}
	return nil
}

func (n *Node) committedUC() *types.UnicityCertificate {
	return n.transactionSystem.CommittedUC()
}

func (n *Node) currentEpoch() uint64 {
	ltr := n.ltr.Load()
	if ltr != nil {
		return ltr.Epoch
	}
	// If we miss LTR then LUC is our best knowledge of current epoch
	return n.luc.Load().InputRecord.Epoch
}

func (n *Node) currentRoundNumber() uint64 {
	ltr := n.ltr.Load()
	if ltr != nil {
		return ltr.Round
	}
	// If we miss LTR then LUC is our best knowledge of current round
	return n.luc.Load().GetRoundNumber() + 1
}

func (n *Node) sendHandshake(ctx context.Context) {
	n.log.DebugContext(ctx, "sending handshake to root chain")
	// select some random root nodes
	rootIDs, err := randomNodeSelector(n.rootNodes, defaultHandshakeNodes)
	if err != nil {
		// error should only happen in case the root nodes are not initialized
		n.log.WarnContext(ctx, "selecting root nodes for handshake", logger.Error(err))
		return
	}
	if err = n.network.Send(ctx,
		handshake.Handshake{
			Partition:      n.configuration.GetPartitionIdentifier(),
			Shard:          n.configuration.shardID,
			NodeIdentifier: n.peer.ID().String(),
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
		return fmt.Errorf("transaction system state does not match unicity certificate, expected '%X', got '%X'", ucIR.Hash, state.Root())
	} else if !bytes.Equal(ucIR.SummaryValue, state.Summary()) {
		return fmt.Errorf("transaction system summary value %X not equal to unicity certificate value %X", ucIR.SummaryValue, state.Summary())
	} else if ucIR.SumOfEarnedFees != sumOfEarnedFees {
		return fmt.Errorf("transaction system sum of earned fees %d not equal to unicity certificate value %d", ucIR.SumOfEarnedFees, sumOfEarnedFees)
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
	for _, txr := range txs {
		txo, err := txr.GetTransactionOrderV1()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to get transaction order: %w", err)
		}
		trx, err := n.validateAndExecuteTx(ctx, txo, round)
		if err != nil {
			n.log.WarnContext(ctx, "processing transaction", logger.Error(err), logger.UnitID(txo.UnitID))
			return nil, 0, fmt.Errorf("processing transaction '%v': %w", txo.UnitID, err)
		}
		sumOfEarnedFees += trx.ServerMetadata.ActualFee
	}
	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return nil, 0, err
	}
	return state, sumOfEarnedFees, nil
}

func getUCv1(b *types.Block) (*types.UnicityCertificate, error) {
	if b == nil {
		return nil, errors.New("block is nil")
	}
	if b.UnicityCertificate == nil {
		return nil, errors.New("block unicity certificate is nil")
	}
	uc := &types.UnicityCertificate{Version: 1}
	return uc, types.Cbor.Unmarshal(b.UnicityCertificate, uc)
}

func (n *Node) restoreBlockProposal(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.restoreBlockProposal")
	defer span.End()

	pr := &types.Block{}
	found, err := n.blockStore.Read(util.Uint32ToBytes(proposalKey), pr)
	if err != nil {
		n.log.ErrorContext(ctx, "Error fetching block proposal", logger.Error(err))
		return
	}
	if !found {
		n.log.DebugContext(ctx, "No pending block proposal stored")
		return
	}
	uc, err := getUCv1(pr)
	if err != nil {
		n.log.WarnContext(ctx, "Error unmarshalling unicity certificate", logger.Error(err))
		return
	}
	// make sure proposal extends the committed state
	if !bytes.Equal(n.committedUC().GetStateHash(), uc.GetPreviousStateHash()) {
		n.log.DebugContext(ctx, "Stored block proposal does not extend previous state, stale proposal")
		return
	}
	// apply stored proposal to current state
	n.log.DebugContext(ctx, "Stored block proposal extends the previous state")
	state, sumOfEarnedFees, err := n.applyBlockTransactions(ctx, uc.GetRoundNumber(), pr.Transactions)
	if err != nil {
		n.log.WarnContext(ctx, "Block proposal recovery failed", logger.Error(err))
		n.revertState()
		return
	}
	if !bytes.Equal(uc.GetStateHash(), state.Root()) {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, state hash mismatch (expected '%X', actual '%X')", uc.InputRecord.Hash, state.Root()))
		n.revertState()
		return
	}
	if !bytes.Equal(uc.GetSummaryValue(), state.Summary()) {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, state summary mismatch (expected '%X', actual '%X')", uc.InputRecord.SummaryValue, state.Summary()))
		n.revertState()
		return
	}
	if uc.GetFeeSum() != sumOfEarnedFees {
		n.log.WarnContext(ctx, fmt.Sprintf("Block proposal transaction failed, sum of earned fees mismatch (expected '%d', actual '%d')", uc.GetFeeSum(), sumOfEarnedFees))
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
	var lastUCReceived = time.Now()
	var lastBlockReceived = time.Now()

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
				n.log.Warn(fmt.Sprintf("handling %T", m), logger.Error(err), logger.Round(n.currentRoundNumber()))
			} else if _, ok := m.(*types.UnicityCertificate); ok {
				lastUCReceived = time.Now()
			} else if _, ok := m.(*types.Block); ok {
				lastBlockReceived = time.Now()
			}
		case <-n.t1event:
			n.handleT1TimeoutEvent(ctx)
		case <-ticker.C:
			n.handleMonitoring(ctx, lastUCReceived, lastBlockReceived)
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
	case *certification.CertificationResponse:
		return n.handleCertificationResponse(ctx, mt)
	case *blockproposal.BlockProposal:
		return n.handleBlockProposal(ctx, mt)
	case *replication.LedgerReplicationRequest:
		return n.handleLedgerReplicationRequest(ctx, mt)
	case *replication.LedgerReplicationResponse:
		return n.handleLedgerReplicationResponse(ctx, mt)
	case *types.Block:
		return n.handleBlock(ctx, mt)
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
	case errors.Is(err, errInvalidPartitionIdentifier):
		return "invalid.sysid"
	default:
		return "err"
	}
}

func (n *Node) process(ctx context.Context, tx *types.TransactionOrder) (rErr error) {
	trx, err := n.validateAndExecuteTx(ctx, tx, n.currentRoundNumber())
	if err != nil || (n.IsFeelessMode() && trx.TxStatus() != types.TxStatusSuccessful) {
		n.sendEvent(event.TransactionFailed, tx)
		if err == nil {
			err = trx.ServerMetadata.ErrDetail()
		}
		return fmt.Errorf("executing transaction %X: %w", tx.Hash(n.configuration.hashAlgorithm), err)
	}
	n.proposedTransactions = append(n.proposedTransactions, trx)
	n.sumOfEarnedFees += trx.GetActualFee()
	n.sendEvent(event.TransactionProcessed, tx)
	n.log.DebugContext(ctx, fmt.Sprintf("transaction processed, proposal size: %d", len(n.proposedTransactions)), logger.UnitID(tx.UnitID))
	return nil
}

func (n *Node) validateAndExecuteTx(ctx context.Context, tx *types.TransactionOrder, round uint64) (_ *types.TransactionRecord, rErr error) {
	defer func(start time.Time) {
		txTypeAttr := attribute.Int("tx", int(tx.Type))
		n.execTxCnt.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(txTypeAttr, attribute.String("status", statusCodeOfTxError(rErr)))))
		n.execTxDur.Record(ctx, time.Since(start).Seconds(), metric.WithAttributeSet(attribute.NewSet(txTypeAttr)))
	}(time.Now())

	if err := n.txValidator.Validate(tx, round); err != nil {
		return nil, fmt.Errorf("invalid transaction: %w", err)
	}
	trx, err := n.transactionSystem.Execute(tx)
	if err != nil {
		return nil, fmt.Errorf("executing transaction in transaction system: %w", err)
	}
	return trx, nil
}

// handleBlockProposal processes a block proposals. Performs the following steps:
//  1. Block proposal as a whole is validated:
//     * It must have valid signature, correct transaction partition ID, valid UC;
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

	n.log.DebugContext(ctx, fmt.Sprintf("Handling block proposal, its UC IR Hash %X, Block hash %X",
		prop.UnicityCertificate.InputRecord.Hash, prop.UnicityCertificate.InputRecord.BlockHash))
	uc := prop.UnicityCertificate
	luc := n.luc.Load()

	// UC must not be older than the last one seen
	if uc.GetRootRoundNumber() < luc.GetRootRoundNumber() {
		return fmt.Errorf("stale block proposal with UC from root round %v, LUC root round %v",
			uc.GetRootRoundNumber(), luc.GetRootRoundNumber())
	}

	// UC can be newer than the last one seen
	if uc.GetRootRoundNumber() > luc.GetRootRoundNumber() {
		// either the other node received it faster from root or there must be some issue with root communication?
		n.log.DebugContext(ctx, fmt.Sprintf("Received newer UC with root round %d via block proposal, LUC root round %d",
			uc.GetRootRoundNumber(), luc.GetRootRoundNumber()))
		if err = n.handleUnicityCertificate(ctx, uc, &prop.Technical); err != nil {
			return fmt.Errorf("block proposal UC handling failed: %w", err)
		}
	}

	// Leader must be the author of the proposal
	expectedLeader := n.leader.Get()
	if expectedLeader == UnknownLeader || prop.NodeIdentifier != expectedLeader.String() {
		return fmt.Errorf("expecting leader %v, leader in proposal: %v", expectedLeader, prop.NodeIdentifier)
	}

	prevHash := uc.InputRecord.Hash
	txState, err := n.transactionSystem.StateSummary()
	if err != nil {
		return fmt.Errorf("transaction system state error, %w", err)
	}
	// check previous state matches before processing transactions
	if !bytes.Equal(prevHash, txState.Root()) {
		return fmt.Errorf("transaction system start state mismatch error, expected: %X, got: %X", txState.Root(), prevHash)
	}
	if err := n.transactionSystem.BeginBlock(n.currentRoundNumber()); err != nil {
		return fmt.Errorf("transaction system BeginBlock error, %w", err)
	}
	for _, tx := range prop.Transactions {
		txo, err := tx.GetTransactionOrderV1()
		if err != nil {
			return fmt.Errorf("failed to get transaction order: %w", err)
		}
		if err = n.process(ctx, txo); err != nil {
			return fmt.Errorf("processing transaction %X: %w", tx.Hash(n.configuration.hashAlgorithm), err)
		}
	}
	if err = n.sendCertificationRequest(ctx, prop.NodeIdentifier); err != nil {
		return fmt.Errorf("certification request send failed, %w", err)
	}
	return nil
}

// Validates the given UC and sets it as the new LUC. Returns an error
// if the UC did not qualify as the new LUC and the node is not in recovery mode.
func (n *Node) updateLUC(ctx context.Context, uc *types.UnicityCertificate, tr *certification.TechnicalRecord) error {
	if uc == nil {
		return fmt.Errorf("unicity certificate is nil")
	}

	// UC is validated cryptographically.
	// TR has already been validated to match the hash in UC
	// when receiving a CertificationResponse or a BlockProposal.
	if err := n.unicityCertificateValidator.Validate(uc); err != nil {
		n.sendEvent(event.Error, err)
		return fmt.Errorf("certificate invalid, %w", err)
	}

	luc := n.luc.Load()
	if n.status.Load() == recovering && luc.GetRootRoundNumber() > uc.GetRootRoundNumber() {
		// During recovery, UC from a recovered block is usually older than the LUC.
		// Do not attempt to update LUC in that case.
		return nil
	}

	if n.status.Load() != initializing {
		n.log.DebugContext(ctx,
			fmt.Sprintf("LUC:\n%s\n\nReceived UC:\n%s", printUC(luc), printUC(uc)),
			logger.Round(n.currentRoundNumber()))
	}

	// check for equivocation
	if err := types.CheckNonEquivocatingCertificates(luc, uc); err != nil {
		// this is not normal, log all info
		n.log.WarnContext(ctx, fmt.Sprintf("equivocating UC for round %d", uc.InputRecord.RoundNumber), logger.Error(err), logger.Data(uc))
		n.log.WarnContext(ctx, "LUC", logger.Data(luc))
		return fmt.Errorf("equivocating certificate: %w", err)
	}

	if uc.IsDuplicate(luc) && n.ltr.Load() != nil {
		// It's OK to receive duplicates, just no need to update luc/ltr if we have both
		return nil
	}

	if tr != nil {
		leaderPeerID, err := peer.Decode(tr.Leader)
		if err != nil {
			return fmt.Errorf("decoding leader peerID from %q: %w", tr.Leader, err)
		}
		n.leader.Set(leaderPeerID)
		n.ltr.Store(tr)
		n.log.DebugContext(ctx, "updated LTR", logger.Round(tr.Round))
	} else {
		n.leader.Set(UnknownLeader)
		n.ltr.Store(nil)
		n.log.DebugContext(ctx, "missing LTR", logger.Round(uc.GetRoundNumber()))
	}

	n.luc.Store(uc)
	n.log.DebugContext(ctx, "updated LUC", logger.Round(uc.GetRoundNumber()))
	n.sendEvent(event.LatestUnicityCertificateUpdated, uc)

	return nil
}

func (n *Node) startNewRound(ctx context.Context) error {
	ctx, span := n.tracer.Start(ctx, "node.startNewRound")
	defer span.End()

	n.resetProposal()
	n.sumOfEarnedFees = 0
	// not a fatal issue, but log anyway
	if err := n.blockStore.Delete(util.Uint32ToBytes(proposalKey)); err != nil {
		n.log.DebugContext(ctx, "DB proposal delete failed", logger.Error(err))
	}

	newRoundNumber := n.currentRoundNumber()
	if n.leader.IsLeader(n.peer.ID()) {
		// followers will start the block once proposal is received
		if err := n.transactionSystem.BeginBlock(newRoundNumber); err != nil {
			return fmt.Errorf("starting new block for round %d: %w", newRoundNumber, err)
		}
		n.leaderCnt.Add(ctx, 1)
	}
	n.startHandleOrForwardTransactions(ctx)
	n.sendEvent(event.NewRoundStarted, newRoundNumber)
	return nil
}

func (n *Node) startRecovery(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.startRecovery")
	defer span.End()

	if n.status.Load() == recovering {
		n.log.DebugContext(ctx, "Recovery already in progress")
		return
	}
	// starting recovery
	n.status.Store(recovering)
	n.revertState()
	n.resetProposal()
	if n.IsValidatorNode() {
		n.stopForwardingOrHandlingTransactions()
	}

	fromBlockNr := n.committedUC().GetRoundNumber() + 1
	n.log.DebugContext(ctx, fmt.Sprintf("Entering recovery state, recover node from %d up to round %d",
		fromBlockNr, n.luc.Load().GetRoundNumber()))
	n.sendEvent(event.RecoveryStarted, fromBlockNr)
	n.sendLedgerReplicationRequest(ctx)
}

func (n *Node) stopRecovery(ctx context.Context) {
	committedBlock := n.committedUC().GetRoundNumber()
	n.log.InfoContext(ctx, fmt.Sprintf("Recovery complete, committed block %d", committedBlock))
	n.sendEvent(event.RecoveryFinished, committedBlock)
	n.status.Store(normal)
}

func (n *Node) isRecoveryComplete() bool {
	return n.committedUC().GetRoundNumber() == n.luc.Load().GetRoundNumber()
}

func (n *Node) handleCertificationResponse(ctx context.Context, cr *certification.CertificationResponse) error {
	if err := cr.IsValid(); err != nil {
		return fmt.Errorf("invalid CertificationResponse: %w", err)
	}
	ctx, span := n.tracer.Start(ctx, "node.handleCertificationResponse", trace.WithAttributes(attribute.Int64("round", int64(cr.Technical.Round))))
	defer span.End()
	n.log.InfoContext(ctx, fmt.Sprintf("handleCertificationResponse: Round %d, Leader %s", cr.Technical.Round, cr.Technical.Leader), logger.Round(n.currentRoundNumber()))

	if cr.Partition != n.PartitionID() || !cr.Shard.Equal(n.configuration.shardID) {
		return fmt.Errorf("got CertificationResponse for a wrong shard %s - %s", cr.Partition, cr.Shard)
	}

	return n.handleUnicityCertificate(ctx, &cr.UC, &cr.Technical)
}

// handleUnicityCertificate processes the Unicity Certificate and finalizes a block. Performs the following steps:
//  1. Given UC is validated cryptographically -> checked before this method is called by unicityCertificateValidator
//  2. Given UC has correct partition identifier -> checked before this method is called by unicityCertificateValidator
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
func (n *Node) handleUnicityCertificate(ctx context.Context, uc *types.UnicityCertificate, tr *certification.TechnicalRecord) error {
	prevLUC := n.luc.Load()

	if err := n.updateLUC(ctx, uc, tr); err != nil {
		return fmt.Errorf("failed to update LUC: %w", err)
	}

	// We have a new or a duplicate luc, let's see what we should do.
	if n.status.Load() == recovering {
		// Doesn't really matter what we got, recovery will handle it.
		n.log.DebugContext(ctx, "Recovery already in progress")
		return nil
	}

	isInitialUC := n.status.Load() == initializing
	if isInitialUC {
		// First UC received after an initial handshake with a root node -> initialization finished.
		n.status.Store(normal)
	}

	if uc.IsDuplicate(prevLUC) {
		// Just ignore duplicates.
		n.log.DebugContext(ctx, "duplicate UC (same root round)", logger.Round(uc.GetRootRoundNumber()))
		if isInitialUC {
			// If this was the first UC received by node, we can start a new round.
			// Otherwise the round is already in progress.
			return n.startNewRound(ctx)
		}
		return nil
	}

	if uc.IsRepeat(prevLUC) {
		// UC certifies the IR before pending block proposal ("repeat UC"). state is rolled back to previous state.
		n.log.WarnContext(ctx, fmt.Sprintf("Reverting state tree on repeat certificate. UC IR hash: %X", uc.GetStateHash()))
		n.revertState()
		return n.startNewRound(ctx)
	}

	committedUC := n.committedUC()
	if !uc.IsSuccessor(committedUC) {
		// Do not allow gaps between blocks, even if state hash does not change.
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, missing blocks. UC previous state hash %X, committed state hash %X",
			uc.GetPreviousStateHash(), committedUC.GetStateHash()))
		n.startRecovery(ctx)
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
			n.startRecovery(ctx)
			return fmt.Errorf("recovery needed, failed to get transaction system state: %w", err)
		}
		// if state hash does not match - start recovery
		if !bytes.Equal(uc.GetStateHash(), state.Root()) {
			n.startRecovery(ctx)
			return ErrNodeDoesNotHaveLatestBlock
		}
		n.log.DebugContext(ctx, "No pending block proposal, UC IR hash is equal to State hash, so are block hashes")
		return n.startNewRound(ctx)
	}
	proposedIR, err := n.pendingBlockProposal.InputRecord()
	if err != nil {
		n.log.WarnContext(ctx, fmt.Sprintf("Invalid block proposal: %v", err))
		n.startRecovery(ctx)
		return ErrNodeDoesNotHaveLatestBlock
	}
	// Check pending block proposal
	n.log.DebugContext(ctx, fmt.Sprintf("Proposed record: %s", proposedIR))
	if err := types.AssertEqualIR(proposedIR, uc.InputRecord); err != nil {
		n.log.WarnContext(ctx, fmt.Sprintf("Recovery needed, received UC does match proposed: %v", err))
		// UC with different IR hash. Node does not have the latest state. Revert changes and start recovery.
		// revertState is called from startRecovery()
		n.startRecovery(ctx)
		return ErrNodeDoesNotHaveLatestBlock
	}
	// replace UC
	n.pendingBlockProposal.UnicityCertificate, err = types.Cbor.Marshal(uc)
	if err != nil {
		return fmt.Errorf("failed to marshal unicity certificate: %w", err)
	}
	// UC certifies pending block proposal
	if err := n.finalizeBlock(ctx, n.pendingBlockProposal, uc); err != nil {
		n.startRecovery(ctx)
		return fmt.Errorf("block %v finalize failed: %w", uc.GetRoundNumber(), err)
	}

	if err = n.network.PublishBlock(ctx, n.pendingBlockProposal); err != nil {
		n.log.WarnContext(ctx, fmt.Sprintf("failed to publish block %d: %v",
			uc.GetRoundNumber(), err))
	}

	return n.startNewRound(ctx)
}

func (n *Node) revertState() {
	n.log.Warn("Reverting state")
	n.sendEvent(event.StateReverted, nil)
	n.transactionSystem.Revert()
	n.sumOfEarnedFees = 0
}

// finalizeBlock creates the block and adds it to the blockStore.
func (n *Node) finalizeBlock(ctx context.Context, b *types.Block, uc *types.UnicityCertificate) error {
	blockNumber := uc.GetRoundNumber()
	_, span := n.tracer.Start(ctx, "Node.finalizeBlock", trace.WithAttributes(attribute.Int64("block.number", int64(blockNumber))))
	defer span.End()

	roundNoInBytes := util.Uint64ToBytes(blockNumber)
	isInitializing := n.status.Load() == initializing

	if !isInitializing {
		// persist the block _before_ committing to tx system
		// if write fails but the round is committed in tx system, there's no way back,
		// but if commit fails, we just remove the block from the store
		if err := n.blockStore.Write(roundNoInBytes, b); err != nil {
			return fmt.Errorf("db write failed, %w", err)
		}
	}

	if err := n.transactionSystem.Commit(uc); err != nil {
		err = fmt.Errorf("unable to finalize block %d: %w", blockNumber, err)

		if !isInitializing {
			if err2 := n.blockStore.Delete(roundNoInBytes); err2 != nil {
				err = errors.Join(err, fmt.Errorf("unable to delete block %d from store: %w", blockNumber, err2))
			}
		}
		return err
	}
	n.sendEvent(event.BlockFinalized, b)

	if isInitializing {
		// ProofIndexer not running yet, index synchronously
		if err := n.proofIndexer.IndexBlock(ctx, b, blockNumber, n.transactionSystem.State()); err != nil {
			return fmt.Errorf("failed to index block: %w", err)
		}
	} else {
		n.proofIndexer.Handle(ctx, b, n.transactionSystem.State())
	}

	if n.ownerIndexer != nil {
		if err := n.ownerIndexer.IndexBlock(b, n.transactionSystem.State()); err != nil {
			return fmt.Errorf("failed to index block: %w", err)
		}
	}
	return nil
}

func (n *Node) handleT1TimeoutEvent(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.handleT1TimeoutEvent", trace.WithNewRoot(), trace.WithAttributes(n.attrRound()))
	defer span.End()

	n.stopForwardingOrHandlingTransactions()

	if n.status.Load() == recovering {
		n.log.InfoContext(ctx, "T1 timeout: node is recovering")
		return
	}
	n.log.InfoContext(ctx, "Handling T1 timeout")
	// if node is not leader, then do not do anything
	if !n.leader.IsLeader(n.peer.ID()) {
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
func (n *Node) handleMonitoring(ctx context.Context, lastUCReceived, lastBlockReceived time.Time) {
	ctx, span := n.tracer.Start(ctx, "node.handleMonitoring", trace.WithNewRoot(), trace.WithAttributes(n.attrRound(), attribute.String("last UC", lastUCReceived.String())))
	defer span.End()

	// check if we have not heard from root validator for T2 timeout + 1 sec
	// a new repeat UC must have been made by now (assuming root is fine) try and get it from other root nodes
	if n.IsValidatorNode() && time.Since(lastUCReceived) > n.configuration.GetT2Timeout()+time.Second {
		// query latest UC from root
		n.sendHandshake(ctx)
	}
	// handle ledger replication timeout - no response from node is received
	if n.status.Load() == recovering && time.Since(n.lastLedgerReqTime) > n.configuration.replicationConfig.timeout {
		n.log.WarnContext(ctx, "Ledger replication timeout, repeat request")
		n.sendLedgerReplicationRequest(ctx)
	} else if !n.IsValidatorNode() && time.Since(lastBlockReceived) > n.configuration.blockSubscriptionTimeout {
		// handle block timeout - no new blocks received
		n.log.WarnContext(ctx, "Block subscription timeout, starting recovery")
		n.startRecovery(ctx)
	}
}

func (n *Node) sendLedgerReplicationResponse(ctx context.Context, msg *replication.LedgerReplicationResponse, toId string) error {
	n.log.DebugContext(ctx, fmt.Sprintf("Sending ledger replication response '%s' to %s: %s", msg.UUID.String(), toId, msg.Pretty()))
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
	n.log.DebugContext(ctx, fmt.Sprintf("Handling ledger replication request '%s' from '%s', starting block %d", lr.UUID.String(), lr.NodeIdentifier, lr.BeginBlockNumber))
	if err := lr.IsValid(); err != nil {
		// for now do not respond to obviously invalid requests
		return fmt.Errorf("invalid request, %w", err)
	}
	if lr.PartitionIdentifier != n.configuration.GetPartitionIdentifier() {
		resp := &replication.LedgerReplicationResponse{
			UUID:    lr.UUID,
			Status:  replication.UnknownPartitionIdentifier,
			Message: fmt.Sprintf("Unknown partition identifier: %s", lr.PartitionIdentifier),
		}
		return n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier)
	}
	startBlock := lr.BeginBlockNumber
	// the node has been started with a later state and does not have the needed data
	if startBlock <= n.fuc.GetRoundNumber() {
		resp := &replication.LedgerReplicationResponse{
			UUID:    lr.UUID,
			Status:  replication.BlocksNotFound,
			Message: fmt.Sprintf("Node does not have block: %v, first block: %v", startBlock, n.fuc.GetRoundNumber()+1),
		}
		return n.sendLedgerReplicationResponse(ctx, resp, lr.NodeIdentifier)
	}
	// the node is behind and does not have the needed data
	latestBlock := n.committedUC().GetRoundNumber()
	if latestBlock < startBlock {
		resp := &replication.LedgerReplicationResponse{
			UUID:    lr.UUID,
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
		var firstFetchedBlockNumber uint64
		var lastFetchedBlockNumber uint64
		var lastFetchedBlock *types.Block
		for ; dbIt.Valid(); dbIt.Next() {
			var bl types.Block
			roundNo := util.BytesToUint64(dbIt.Key())
			if err := dbIt.Value(&bl); err != nil {
				n.log.WarnContext(ctx, fmt.Sprintf("Ledger replication reply incomplete, failed to read block %d", roundNo), logger.Error(err))
				break
			}
			lastFetchedBlock = &bl
			if firstFetchedBlockNumber == 0 {
				firstFetchedBlockNumber = roundNo
			}
			lastFetchedBlockNumber = roundNo
			blocks = append(blocks, lastFetchedBlock)
			blockCnt++
			countTx += uint32(len(bl.Transactions))
			if countTx >= n.configuration.replicationConfig.maxTx ||
				blockCnt >= n.configuration.replicationConfig.maxReturnBlocks ||
				(roundNo >= lr.EndBlockNumber && lr.EndBlockNumber > 0) {
				break
			}
		}
		resp := &replication.LedgerReplicationResponse{
			UUID:             lr.UUID,
			Status:           replication.Ok,
			Blocks:           blocks,
			FirstBlockNumber: firstFetchedBlockNumber,
			LastBlockNumber:  lastFetchedBlockNumber,
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
	n.log.DebugContext(ctx, fmt.Sprintf("Ledger replication response '%s' received: %s, ", lr.UUID.String(), lr.Pretty()))
	if lr.Status != replication.Ok {
		// In case recovery was caused by a timeout, we can return to normal mode as long as we have all known blocks
		if n.isRecoveryComplete() {
			n.stopRecovery(ctx)
		}
		return fmt.Errorf("received error response, status=%s, message='%s'", lr.Status.String(), lr.Message)
	}

	// check for duplicate requests:
	// if we have already seen the first block in the replication response then the replication must have timed out and
	// multiple replication requests must have been performed, discard the last arrived duplicate batch
	lastCommittedRoundNumber := n.committedUC().GetRoundNumber()
	if lr.FirstBlockNumber <= lastCommittedRoundNumber {
		n.log.DebugContext(ctx, fmt.Sprintf("Duplicate Ledger Replication response, received blocks %d to %d but have latest committed block %d (replication timed out and node sent multiple replication requests?): %s", lr.FirstBlockNumber, lr.LastBlockNumber, lastCommittedRoundNumber, lr.Pretty()))
		return nil
	}

	for _, b := range lr.Blocks {
		if err := n.handleBlock(ctx, b); err != nil {
			return err
		}
	}

	if !n.isRecoveryComplete() {
		n.log.DebugContext(ctx, fmt.Sprintf("Recovery incomplete, committed block %d vs available block %d",
			n.committedUC().GetRoundNumber(), n.luc.Load().GetRoundNumber()))
		n.sendLedgerReplicationRequest(ctx)
		return nil
	}

	n.stopRecovery(ctx)

	if n.IsValidatorNode() {
		if err := n.startNewRound(ctx); err != nil {
			return err
		}
	}

	// try to apply the last received block proposal received during recovery,
	// it may fail if the block was finalized and is in fact the last block received
	if n.recoveryLastProp != nil {
		// try to apply it to the latest state, may fail
		if err := n.handleBlockProposal(ctx, n.recoveryLastProp); err != nil {
			n.log.DebugContext(ctx, "Recovery completed, failed to apply last received block proposal(stale?)", logger.Error(err))
		}
		n.recoveryLastProp = nil
	}

	return nil
}

func (n *Node) handleBlock(ctx context.Context, b *types.Block) error {
	committedUC := n.committedUC()
	blockUC, err := getUCv1(b)
	if err != nil {
		return fmt.Errorf("failed to extract UC from block: %w", err)
	}
	algo := n.configuration.hashAlgorithm
	if err := b.IsValid(algo, n.configuration.genesis.PartitionDescription.Hash(algo)); err != nil {
		// sends invalid blocks, do not trust the response and try again
		return fmt.Errorf("invalid block for round %v: %w", blockUC.GetRoundNumber(), err)
	}

	// LUC is the latest UC we have seen, it's ok to update it as soon as we see it.
	// TechnicalRecord not available, we might have a wrong idea of the current round/epoch.
	if err := n.updateLUC(ctx, blockUC, nil); err != nil {
		return fmt.Errorf("failed to update LUC: %w", err)
	}

	// it could be that we receive blocks from earlier time or later time, make sure to extend from what is missing
	if blockUC.GetRoundNumber() <= committedUC.GetRoundNumber() {
		n.log.DebugContext(ctx, fmt.Sprintf("latest committed block %v, skipping block %v", committedUC.GetRoundNumber(), blockUC.GetRoundNumber()))
		return nil
	} else if !blockUC.IsSuccessor(committedUC) {
		// No point in starting recovery during initialization - node won't start. Perhaps it should?
		if n.status.Load() != initializing {
			n.startRecovery(ctx)
		}
		return fmt.Errorf("missing blocks between rounds %v and %v", committedUC.GetRoundNumber(), blockUC.GetRoundNumber())
	}

	if !bytes.Equal(b.Header.PreviousBlockHash, committedUC.InputRecord.BlockHash) {
		return fmt.Errorf("invalid block %v (expected previous block hash='%X', actual previous block hash='%X', )",
			blockUC.GetRoundNumber(), b.Header.PreviousBlockHash, committedUC.InputRecord.BlockHash)
	}

	n.log.DebugContext(ctx, fmt.Sprintf("Applying block from round %d", blockUC.GetRoundNumber()))

	// make sure it extends current state
	var state txsystem.StateSummary
	state, err = n.transactionSystem.StateSummary()
	if err != nil {
		return fmt.Errorf("error reading current state, %w", err)
	}
	if !bytes.Equal(blockUC.InputRecord.PreviousHash, state.Root()) {
		return fmt.Errorf("block does not extend current state, expected state hash: %X, actual state hash: %X",
			blockUC.InputRecord.PreviousHash, state.Root())
	}

	var sumOfEarnedFees uint64
	state, sumOfEarnedFees, err = n.applyBlockTransactions(ctx, blockUC.GetRoundNumber(), b.Transactions)
	if err != nil {
		n.revertState()
		return fmt.Errorf("failed to apply block %v transactions: %w", blockUC.GetRoundNumber(), err)
	}

	if err = verifyTxSystemState(state, sumOfEarnedFees, blockUC.InputRecord); err != nil {
		n.revertState()
		return fmt.Errorf("failed to verify block %v state: %w", blockUC.GetRoundNumber(), err)
	}

	if err = n.finalizeBlock(ctx, b, blockUC); err != nil {
		// TODO: Should we revert in case only indexing failed?
		n.revertState()
		return fmt.Errorf("failed to finalize block %v: %w", blockUC.GetRoundNumber(), err)
	}

	return nil
}

func (n *Node) sendLedgerReplicationRequest(ctx context.Context) {
	startingBlockNr := n.committedUC().GetRoundNumber() + 1
	ctx, span := n.tracer.Start(ctx, "node.sendLedgerReplicationRequest", trace.WithAttributes(attribute.Int64("starting_block", int64(startingBlockNr))))
	defer span.End()

	req := &replication.LedgerReplicationRequest{
		UUID:                uuid.New(),
		PartitionIdentifier: n.configuration.GetPartitionIdentifier(),
		NodeIdentifier:      n.peer.ID().String(),
		BeginBlockNumber:    startingBlockNr,
		EndBlockNumber:      startingBlockNr + n.configuration.replicationConfig.maxFetchBlocks,
	}
	n.log.Log(ctx, logger.LevelTrace, "sending ledger replication request", logger.Data(req))

	// TODO: should send to non-validators also
	peers := n.validatorNodes
	if len(peers) == 0 {
		n.log.WarnContext(ctx, "Error sending ledger replication request, no peers")
		return
	}

	// send Ledger Replication request to a first alive randomly chosen node
	for _, p := range util.ShuffleSliceCopy(peers) {
		if n.peer.ID() == p {
			continue
		}
		n.log.DebugContext(ctx, fmt.Sprintf("Sending ledger replication request '%s' to %v", req.UUID.String(), p))
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

	ltr := n.ltr.Load()
	if ltr == nil {
		// Should not reach here, leader is unknown without LTR
		return fmt.Errorf("missing LTR")
	}

	nodeId := n.peer.ID()
	prop := &blockproposal.BlockProposal{
		Partition:          n.configuration.GetPartitionIdentifier(),
		Shard:              n.configuration.shardID,
		NodeIdentifier:     nodeId.String(),
		UnicityCertificate: n.luc.Load(),
		Technical:          *ltr,
		Transactions:       n.proposedTransactions,
	}
	n.log.Log(ctx, logger.LevelTrace, "created BlockProposal", logger.Data(prop))
	if err := prop.Sign(n.configuration.hashAlgorithm, n.configuration.signer); err != nil {
		return fmt.Errorf("block proposal sign failed, %w", err)
	}
	n.blockSize.Add(ctx, int64(len(prop.Transactions)))
	return n.network.Send(ctx, prop, n.FilterValidatorNodes(nodeId)...)
}

func (n *Node) persistBlockProposal(pr *types.Block) error {
	if err := n.blockStore.Write(util.Uint32ToBytes(proposalKey), pr); err != nil {
		return fmt.Errorf("persist error, %w", err)
	}
	return nil
}

func (n *Node) sendCertificationRequest(ctx context.Context, blockAuthor string) error {
	ctx, span := n.tracer.Start(ctx, "node.sendCertificationRequest", trace.WithAttributes(attribute.String("author", blockAuthor)))
	defer span.End()

	state, err := n.transactionSystem.EndBlock()
	if err != nil {
		return fmt.Errorf("transaction system failed to end block: %w", err)
	}
	luc := n.luc.Load()
	stateHash := state.Root()
	uc := &types.UnicityCertificate{
		Version: 1,
		InputRecord: &types.InputRecord{
			Version:         1,
			Epoch:           n.currentEpoch(),
			RoundNumber:     n.currentRoundNumber(),
			PreviousHash:    luc.GetStateHash(),
			Hash:            stateHash,
			Timestamp:       luc.UnicitySeal.Timestamp,
			SummaryValue:    state.Summary(),
			SumOfEarnedFees: n.sumOfEarnedFees,
		},
	}
	ucBytes, err := types.Cbor.Marshal(uc)
	if err != nil {
		return fmt.Errorf("failed to marshal unicity certificate: %w", err)
	}
	pendingProposal := &types.Block{
		Header: &types.Header{
			Version:           1,
			PartitionID:       n.configuration.GetPartitionIdentifier(),
			ShardID:           n.configuration.shardID,
			ProposerID:        blockAuthor,
			PreviousBlockHash: n.committedUC().InputRecord.BlockHash,
		},
		Transactions:       n.proposedTransactions,
		UnicityCertificate: ucBytes,
	}
	ir, err := pendingProposal.CalculateBlockHash(n.configuration.hashAlgorithm)
	if err != nil {
		return fmt.Errorf("calculating block hash: %w", err)
	}
	if err = n.persistBlockProposal(pendingProposal); err != nil {
		n.transactionSystem.Revert()
		return fmt.Errorf("failed to store pending block proposal: %w", err)
	}
	n.pendingBlockProposal = pendingProposal
	n.proposedTransactions = []*types.TransactionRecord{}
	n.sumOfEarnedFees = 0
	// send new input record for certification
	req := &certification.BlockCertificationRequest{
		Partition:      n.configuration.GetPartitionIdentifier(),
		Shard:          n.configuration.shardID,
		NodeIdentifier: n.peer.ID().String(),
		InputRecord:    ir,
	}
	if req.BlockSize, err = pendingProposal.Size(); err != nil {
		return fmt.Errorf("calculating block size: %w", err)
	}
	if req.StateSize, err = n.transactionSystem.StateSize(); err != nil {
		return fmt.Errorf("calculating state size: %w", err)
	}
	if err = req.Sign(n.configuration.signer); err != nil {
		return fmt.Errorf("failed to sign certification request: %w", err)
	}
	n.log.InfoContext(ctx, fmt.Sprintf("Round %v sending block certification request to root chain, IR hash %X, Block Hash %X, fee sum %d",
		uc.GetRoundNumber(), stateHash, ir.BlockHash, uc.GetFeeSum()))
	n.log.Log(ctx, logger.LevelTrace, "Block Certification req", logger.Data(req))
	rootIDs, err := rootNodesSelector(luc, n.rootNodes, defaultNofRootNodes)
	if err != nil {
		return fmt.Errorf("selecting root nodes: %w", err)
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

func (n *Node) GetTransactionRecordProof(ctx context.Context, txoHash []byte) (*types.TxRecordProof, error) {
	proofs := n.proofIndexer.GetDB()
	index, err := ReadTransactionIndex(proofs, txoHash)
	if err != nil {
		return nil, fmt.Errorf("unable to query tx index: %w", err)
	}
	b, err := n.GetBlock(ctx, index.RoundNumber)
	if err != nil {
		return nil, fmt.Errorf("unable to load block: %w", err)
	}
	txRecordProof, err := types.NewTxRecordProof(b, index.TxOrderIndex, n.configuration.hashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("unable to extract transaction record and execution proof from the block: %w", err)
	}
	txo, err := txRecordProof.GetTransactionOrderV1()
	if err != nil {
		return nil, fmt.Errorf("unable to extract transaction order from the block: %w", err)
	}
	h := txo.Hash(n.configuration.hashAlgorithm)
	if !bytes.Equal(h, txoHash) {
		return nil, errors.New("transaction index is invalid: hash mismatch")
	}
	return txRecordProof, nil
}

func (n *Node) NetworkID() types.NetworkID {
	return n.configuration.GetNetworkIdentifier()
}

func (n *Node) PartitionID() types.PartitionID {
	return n.configuration.GetPartitionIdentifier()
}

func (n *Node) Peer() *network.Peer {
	return n.peer
}

func (n *Node) ValidatorNodes() peer.IDSlice {
	return n.validatorNodes
}

func (n *Node) IsValidatorNode() bool {
	return slices.Contains(n.validatorNodes, n.Peer().ID())
}

func (n *Node) IsPermissionedMode() bool {
	return n.transactionSystem.IsPermissionedMode()
}

func (n *Node) IsFeelessMode() bool {
	return n.transactionSystem.IsFeelessMode()
}

func (n *Node) CurrentRoundNumber(ctx context.Context) (uint64, error) {
	_, span := n.tracer.Start(ctx, "node.CurrentRoundNumber")
	defer span.End()
	if status := n.status.Load(); status != normal {
		return 0, fmt.Errorf("node not ready: %s", status)
	}
	return n.currentRoundNumber(), nil
}

func (n *Node) GetTrustBase(epochNumber uint64) (types.RootTrustBase, error) {
	// TODO verify epoch number after epoch switching is implemented
	// fast-track solution is to restart all partition nodes with new config on epoch change
	trustBase := n.configuration.trustBase
	if trustBase == nil {
		return nil, fmt.Errorf("trust base for epoch %d does not exist", epochNumber)
	}
	return trustBase, nil
}

func (n *Node) FilterValidatorNodes(exclude peer.ID) []peer.ID {
	var result []peer.ID
	for _, v := range n.validatorNodes {
		if v != exclude {
			result = append(result, v)
		}
	}
	return result
}

func (n *Node) TransactionSystemState() txsystem.StateReader {
	return n.transactionSystem.State()
}

func (n *Node) stopForwardingOrHandlingTransactions() {
	n.stopTxProcessor.Load().(func())()
}

func (n *Node) startHandleOrForwardTransactions(ctx context.Context) {
	ctx, span := n.tracer.Start(ctx, "node.startHandleOrForwardTransactions", trace.WithAttributes(n.attrRound()))
	defer span.End()

	n.stopForwardingOrHandlingTransactions()

	leader := n.leader.Get()
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
			n.network.ForwardTransactions(ctx, n.leader.Get)
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

func (n *Node) attrRound() attribute.KeyValue {
	return observability.Round(n.currentRoundNumber())
}

func printUC(uc *types.UnicityCertificate) string {
	return fmt.Sprintf("H:\t%X\nH':\t%X\nHb:\t%X\nfees:%d, round:%d, root round:%d",
		uc.InputRecord.Hash, uc.InputRecord.PreviousHash, uc.InputRecord.BlockHash,
		uc.InputRecord.SumOfEarnedFees, uc.GetRoundNumber(), uc.GetRootRoundNumber())
}
