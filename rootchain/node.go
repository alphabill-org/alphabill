package rootchain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/observability"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
)

type (
	PartitionNet interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Tracer(name string, options ...trace.TracerOption) trace.Tracer
		Logger() *slog.Logger
	}

	ConsensusManager interface {
		// RequestCertification accepts certification requests with proof of quorum or no-quorum.
		RequestCertification(ctx context.Context, cr consensus.IRChangeRequest) error
		// CertificationResult read the channel to receive certification results
		CertificationResult() <-chan *certification.CertificationResponse
		ShardInfo(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error)
		// Run consensus algorithm
		Run(ctx context.Context) error
	}

	Node struct {
		peer             *network.Peer // p2p network host for partition
		incomingRequests *CertRequestBuffer
		subscription     *Subscriptions
		net              PartitionNet
		consensusManager ConsensusManager

		log    *slog.Logger
		tracer trace.Tracer

		execMsgCnt metric.Int64Counter
	}
)

// New creates a new instance of the root chain node
func New(
	peer *network.Peer,
	pNet PartitionNet,
	cm ConsensusManager,
	observe Observability,
) (*Node, error) {
	if peer == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}

	meter := observe.Meter("rootchain.node")
	reqBuf, err := NewCertificationRequestBuffer(meter)
	if err != nil {
		return nil, fmt.Errorf("creating request buffer: %w", err)
	}
	subs, err := NewSubscriptions(pNet.Send, observe)
	if err != nil {
		return nil, fmt.Errorf("creating subscribers list: %w", err)
	}
	node := &Node{
		peer:             peer,
		incomingRequests: reqBuf,
		subscription:     subs,
		net:              pNet,
		consensusManager: cm,
		log:              observe.Logger(),
		tracer:           observe.Tracer("rootchain.node"),
	}
	if err := node.initMetrics(meter); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}
	return node, nil
}

func (v *Node) initMetrics(m metric.Meter) (err error) {
	v.execMsgCnt, err = m.Int64Counter("exec.msg.count", metric.WithDescription("Number of messages processed by the node"))
	if err != nil {
		return fmt.Errorf("creating counter for processed messages: %w", err)
	}

	return nil
}

func (v *Node) Run(ctx context.Context) error {
	v.log.InfoContext(ctx, fmt.Sprintf("Starting root node. Addresses=%v; BuildInfo=%s", v.peer.MultiAddresses(), debug.ReadBuildInfo()))
	g, gctx := errgroup.WithContext(ctx)
	// Run root consensus algorithm
	g.Go(func() error { return v.consensusManager.Run(gctx) })
	// Start receiving messages from partition nodes
	g.Go(func() error { return v.partitionMsgLoop(gctx) })
	// Start handling certification responses
	g.Go(func() error { return v.handleConsensus(gctx) })
	return g.Wait()
}

func (v *Node) GetPeer() *network.Peer {
	return v.peer
}

// handle messages from partition nodes
func (v *Node) partitionMsgLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-v.net.ReceivedChannel():
			if !ok {
				return fmt.Errorf("partition channel closed")
			}
			v.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("received %T", msg), logger.Data(msg))
			if err := v.handlePartitionMsg(ctx, msg); err != nil {
				v.log.WarnContext(ctx, fmt.Sprintf("processing %T", msg), logger.Error(err))
			}
		}
	}
}

func (v *Node) handlePartitionMsg(ctx context.Context, msg any) (rErr error) {
	partitionID, shardID, nodeID := types.PartitionID(0), types.ShardID{}, ""
	ctx, span := v.tracer.Start(ctx, "node.handlePartitionMsg")
	defer func() {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		msgAttr := attribute.String("msg", fmt.Sprintf("%T", msg))
		attrNode := attribute.String("node.id", nodeID)
		v.execMsgCnt.Add(ctx, 1, observability.Shard(partitionID, shardID, msgAttr, attrNode, observability.ErrStatus(rErr)))
		span.SetAttributes(msgAttr, observability.Partition(partitionID), attrNode)
		span.End()
	}()

	switch mt := msg.(type) {
	case *certification.BlockCertificationRequest:
		partitionID, shardID, nodeID = mt.PartitionID, mt.ShardID, mt.NodeID
		return v.onBlockCertificationRequest(ctx, mt)
	case *handshake.Handshake:
		partitionID, shardID, nodeID = mt.PartitionID, mt.ShardID, mt.NodeID
		return v.onHandshake(ctx, mt)
	default:
		return fmt.Errorf("unknown message type %T", msg)
	}
}

func (v *Node) sendResponse(ctx context.Context, nodeID string, cr *certification.CertificationResponse) error {
	ctx, span := v.tracer.Start(ctx, "node.sendResponse")
	defer span.End()

	peerID, err := peer.Decode(nodeID)
	if err != nil {
		return fmt.Errorf("invalid receiver id: %w", err)
	}

	if err := cr.IsValid(); err != nil {
		return fmt.Errorf("invalid certification response: %w", err)
	}
	return v.net.Send(ctx, cr, peerID)
}

func (v *Node) onHandshake(ctx context.Context, req *handshake.Handshake) error {
	ctx, span := v.tracer.Start(ctx, "node.onHandshake")
	defer span.End()

	if err := req.IsValid(); err != nil {
		return fmt.Errorf("invalid handshake request: %w", err)
	}
	si, err := v.consensusManager.ShardInfo(req.PartitionID, req.ShardID)
	if err != nil {
		return fmt.Errorf("reading partition %s certificate: %w", req.PartitionID, err)
	}
	// verifies nodeID is part of active validator set
	err = si.Verify(req.NodeID, func(v abcrypto.Verifier) error {
		return nil
	})
	if err != nil {
		return fmt.Errorf("node ID is not in active validator set %s - %s - %s", req.PartitionID, req.ShardID, req.NodeID)
	}

	if si.LastCR.UC.GetRoundNumber() == 0 {
		// Make sure shard nodes get CertificationResponses even
		// before they send the first BlockCertificationRequests
		if err := v.subscription.Subscribe(req.PartitionID, req.ShardID, req.NodeID); err != nil {
			return fmt.Errorf("subscribing the sender: %w", err)
		}
	}
	if err = v.sendResponse(ctx, req.NodeID, si.LastCR); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	return nil
}

/*
onBlockCertificationRequest handles Certification Request from shard nodes.
Shard nodes can only extend the stored/certified state.
*/
func (v *Node) onBlockCertificationRequest(ctx context.Context, req *certification.BlockCertificationRequest) (rErr error) {
	ctx, span := v.tracer.Start(ctx, "node.onBlockCertificationRequest")
	defer span.End()

	si, err := v.consensusManager.ShardInfo(req.PartitionID, req.ShardID)
	if err != nil {
		return fmt.Errorf("acquiring shard %s - %s info: %w", req.PartitionID, req.ShardID, err)
	}
	if err := si.ValidRequest(req); err != nil {
		err = fmt.Errorf("invalid block certification request: %w", err)
		if se := v.sendResponse(ctx, req.NodeID, si.LastCR); se != nil {
			err = errors.Join(err, fmt.Errorf("sending latest cert: %w", se))
		}
		return err
	}

	// we got the shard info thus it's a valid partition/shard
	if err := v.subscription.Subscribe(req.PartitionID, req.ShardID, req.NodeID); err != nil {
		return fmt.Errorf("subscribing the sender: %w", err)
	}

	// check if consensus is already achieved
	if res := v.incomingRequests.IsConsensusReceived(req.PartitionID, req.ShardID, si); res != QuorumInProgress {
		v.log.DebugContext(ctx, fmt.Sprintf("dropping stale block certification request (%s) for partition %s", res, req.PartitionID), logger.Shard(req.PartitionID, req.ShardID))
		return
	}
	// store the new request and see if quorum is now achieved
	res, requests, err := v.incomingRequests.Add(ctx, req, si)
	if err != nil {
		return fmt.Errorf("storing request: %w", err)
	}
	var reason consensus.CertReqReason
	switch res {
	case QuorumAchieved:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s reached consensus, new InputHash: %X", req.PartitionID, requests[0].InputRecord.Hash), logger.Shard(req.PartitionID, req.ShardID))
		reason = consensus.Quorum
	case QuorumNotPossible:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s consensus not possible, repeat UC", req.PartitionID), logger.Shard(req.PartitionID, req.ShardID))
		reason = consensus.QuorumNotPossible
	case QuorumInProgress:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s quorum not yet reached, but possible in the future", req.PartitionID), logger.Shard(req.PartitionID, req.ShardID))
		return nil
	}

	if err = v.consensusManager.RequestCertification(ctx,
		consensus.IRChangeRequest{
			Partition: req.PartitionID,
			Shard:     req.ShardID,
			Reason:    reason,
			Requests:  requests,
		}); err != nil {
		v.incomingRequests.Clear(ctx, req.PartitionID, req.ShardID)
		return fmt.Errorf("requesting certification: %w", err)
	}
	return nil
}

// handleConsensus - receives consensus results and delivers certificates to subscribers
func (v *Node) handleConsensus(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case cr, ok := <-v.consensusManager.CertificationResult():
			if !ok {
				return fmt.Errorf("consensus channel closed")
			}
			v.subscription.Send(ctx, cr)
			v.incomingRequests.Clear(ctx, cr.Partition, cr.Shard)
		}
	}
}
