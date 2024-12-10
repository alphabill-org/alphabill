package rootchain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

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

		bcrCount   metric.Int64Counter // Block Certification Request count
		bcRespSent metric.Int64Counter // Block Certification Responses sent
	}
)

// New creates a new instance of the root chain node
func New(
	p *network.Peer,
	pNet PartitionNet,
	cm ConsensusManager,
	observe Observability,
) (*Node, error) {
	if p == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}

	meter := observe.Meter("rootchain.node", metric.WithInstrumentationAttributes(observability.PeerID("node.id", p.ID())))
	node := &Node{
		peer:             p,
		incomingRequests: NewCertificationRequestBuffer(),
		subscription:     NewSubscriptions(meter),
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
	v.bcrCount, err = m.Int64Counter("block.cert.req", metric.WithDescription("Number of Block Certification Requests received"))
	if err != nil {
		return fmt.Errorf("creating Block Certification Requests counter: %w", err)
	}
	v.bcRespSent, err = m.Int64Counter("block.cert.rsp", metric.WithDescription("Number of Block Certification Responses sent (ie how many subscribers the node had)"))
	if err != nil {
		return fmt.Errorf("creating Block Certification Responses counter: %w", err)
	}

	return nil
}

func (v *Node) Run(ctx context.Context) error {
	v.log.InfoContext(ctx, fmt.Sprintf("Starting root node. Addresses=%v; BuildInfo=%s", v.peer.MultiAddresses(), debug.ReadBuildInfo()))
	g, gctx := errgroup.WithContext(ctx)
	// Run root consensus algorithm
	g.Go(func() error { return v.consensusManager.Run(gctx) })
	// Start receiving messages from partition nodes
	g.Go(func() error { return v.loop(gctx) })
	// Start handling certification responses
	g.Go(func() error { return v.handleConsensus(gctx) })
	return g.Wait()
}

func (v *Node) GetPeer() *network.Peer {
	return v.peer
}

// loop handles messages from different goroutines.
func (v *Node) loop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-v.net.ReceivedChannel():
			if !ok {
				return fmt.Errorf("partition channel closed")
			}
			v.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("received %T", msg), logger.Data(msg))
			switch mt := msg.(type) {
			case *certification.BlockCertificationRequest:
				if err := v.onBlockCertificationRequest(ctx, mt); err != nil {
					v.log.LogAttrs(ctx, slog.LevelWarn, fmt.Sprintf("handling block certification request from %s", mt.NodeID), logger.Error(err))
				}
			case *handshake.Handshake:
				if err := v.onHandshake(ctx, mt); err != nil {
					v.log.LogAttrs(ctx, slog.LevelWarn, fmt.Sprintf("handling handshake from %s", mt.NodeID), logger.Error(err))
				}
			default:
				v.log.LogAttrs(ctx, slog.LevelWarn, fmt.Sprintf("message %T not supported.", msg))
			}
		}
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
	if err := req.IsValid(); err != nil {
		return fmt.Errorf("invalid handshake request: %w", err)
	}
	si, err := v.consensusManager.ShardInfo(req.PartitionID, req.ShardID)
	if err != nil {
		return fmt.Errorf("reading partition %s certificate: %w", req.PartitionID, err)
	}
	if err = v.sendResponse(ctx, req.NodeID, si.LastCR); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	return nil
}

/*
onBlockCertificationRequest handles Certification Request from partition nodes.
Partition nodes can only extend the stored/certified state.
*/
func (v *Node) onBlockCertificationRequest(ctx context.Context, req *certification.BlockCertificationRequest) (rErr error) {
	ctx, span := v.tracer.Start(ctx, "node.onBlockCertificationRequest")
	defer func() {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		span.SetAttributes(observability.Partition(req.PartitionID))
		v.bcrCount.Add(ctx, 1, observability.Shard(req.PartitionID, req.ShardID, observability.ErrStatus(rErr)))
		span.End()
	}()

	si, err := v.consensusManager.ShardInfo(req.PartitionID, req.ShardID)
	if err != nil {
		return fmt.Errorf("acquiring shard %s - %s info: %w", req.PartitionID, req.ShardID, err)
	}
	v.subscription.Subscribe(req.PartitionID, req.NodeID)
	// we got the shard info thus it's a valid partition/shard
	if err := si.ValidRequest(req); err != nil {
		err = fmt.Errorf("invalid block certification request: %w", err)
		if se := v.sendResponse(ctx, req.NodeID, si.LastCR); se != nil {
			err = errors.Join(err, fmt.Errorf("sending latest cert: %w", se))
		}
		return err
	}

	// check if consensus is already achieved
	if res := v.incomingRequests.IsConsensusReceived(req.PartitionID, req.ShardID, si); res != QuorumInProgress {
		v.log.DebugContext(ctx, fmt.Sprintf("dropping stale block certification request (%s) for partition %s", res, req.PartitionID), logger.Shard(req.PartitionID, req.ShardID))
		return
	}
	// store the new request and see if quorum is now achieved
	res, requests, err := v.incomingRequests.Add(req, si)
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
		case uc, ok := <-v.consensusManager.CertificationResult():
			if !ok {
				return fmt.Errorf("consensus channel closed")
			}
			v.onCertificationResult(ctx, uc)
		}
	}
}

func (v *Node) onCertificationResult(ctx context.Context, cr *certification.CertificationResponse) {
	// remember to clear the incoming buffer to accept new nodeRequest
	// NB! this will try and reset the store also in the case when partition id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(cr.Partition, cr.Shard)
		v.log.LogAttrs(ctx, logger.LevelTrace, "Resetting request store for shard", logger.Shard(cr.Partition, cr.Shard))
	}()

	subscribed := v.subscription.Get(cr.Partition)
	v.log.DebugContext(ctx, fmt.Sprintf("sending CertificationResponse, %d receivers, R_next: %d, IR Hash: %X, Block Hash: %X",
		len(subscribed), cr.Technical.Round, cr.UC.InputRecord.Hash, cr.UC.InputRecord.BlockHash), logger.Shard(cr.Partition, cr.Shard))
	v.bcRespSent.Add(ctx, int64(len(subscribed)), observability.Shard(cr.Partition, cr.Shard))
	// send response to all registered nodes
	for _, node := range subscribed {
		if err := v.sendResponse(ctx, node, cr); err != nil {
			v.log.WarnContext(ctx, "sending certification result", logger.Error(err), logger.Shard(cr.Partition, cr.Shard))
		}
		v.subscription.ResponseSent(cr.Partition, node)
	}
}
