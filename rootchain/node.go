package rootchain

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
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
	"github.com/alphabill-org/alphabill/rootchain/partitions"
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
		// GetLatestUnicityCertificate get the latest certification for partition (maybe should/can be removed)
		GetLatestUnicityCertificate(id types.SystemID, shard types.ShardID) (*certification.CertificationResponse, error)
		// Run consensus algorithm
		Run(ctx context.Context) error
	}

	Node struct {
		peer             *network.Peer // p2p network host for partition
		partitions       partitions.PartitionConfiguration
		incomingRequests *CertRequestBuffer
		subscription     *Subscriptions
		net              PartitionNet
		consensusManager ConsensusManager
		shardInfo        map[partitionShard]*shardInfo

		log    *slog.Logger
		tracer trace.Tracer

		bcrCount metric.Int64Counter // Block Certification Request count
	}
)

// New creates a new instance of the root chain node
func New(
	p *network.Peer,
	pNet PartitionNet,
	ps partitions.PartitionConfiguration,
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
		partitions:       ps,
		incomingRequests: NewCertificationRequestBuffer(),
		subscription:     NewSubscriptions(meter),
		net:              pNet,
		consensusManager: cm,
		shardInfo:        make(map[partitionShard]*shardInfo),
		log:              observe.Logger(),
		tracer:           observe.Tracer("rootchain.node"),
	}
	if err := node.initMetrics(meter); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}
	return node, nil
}

func (v *Node) initMetrics(m metric.Meter) (err error) {
	v.bcrCount, err = m.Int64Counter("block.cert.req", metric.WithDescription("Number of Block Certification Requests processed"))
	if err != nil {
		return fmt.Errorf("creating Block Certification Requests counter: %w", err)
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
					v.log.LogAttrs(ctx, slog.LevelWarn, fmt.Sprintf("handling block certification request from %s", mt.NodeIdentifier), logger.Error(err))
				}
			case *handshake.Handshake:
				if err := v.onHandshake(ctx, mt); err != nil {
					v.log.LogAttrs(ctx, slog.LevelWarn, fmt.Sprintf("handling handshake from %s", mt.NodeIdentifier), logger.Error(err))
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
	// TODO: send the CertResponse instead of just UC! (AB-1725)
	return v.net.Send(ctx, &cr.UC, peerID)
}

func (v *Node) onHandshake(ctx context.Context, req *handshake.Handshake) error {
	if err := req.IsValid(); err != nil {
		return fmt.Errorf("invalid handshake request: %w", err)
	}
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(req.Partition, req.Shard)
	if err != nil {
		return fmt.Errorf("reading partition %s certificate: %w", req.Partition, err)
	}
	if err = v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); err != nil {
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
	defer span.End()

	sysID := req.Partition
	if sysID == 0 {
		v.bcrCount.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.String("status", "err.sysid"))))
		return fmt.Errorf("request contains invalid partition identifier %s", sysID)
	}
	defer func() {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		partition := observability.Partition(sysID)
		span.SetAttributes(partition)
		v.bcrCount.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(observability.ErrStatus(rErr), partition)))
	}()

	// todo: we need shard TrustBase but as we only support single shard
	// partitions "partition TB == shard TB" (AB-1716)
	// Also, the TB should be part of the ShardInfo, to be refactored
	// with epoch/conf change support...
	pdr, pTrustBase, err := v.partitions.GetInfo(sysID, req.RootRound())
	if err != nil {
		return fmt.Errorf("reading partition info: %w", err)
	}
	if err := pdr.IsValidShard(req.Shard); err != nil {
		return fmt.Errorf("invalid shard: %w", err)
	}
	if err = pTrustBase.Verify(req.NodeIdentifier, req); err != nil {
		return fmt.Errorf("partition %s node %v rejected: %w", sysID, req.NodeIdentifier, err)
	}

	// we need the leader ID so we require that it's sent to us as part of the request
	// however, per YP rootchain should select the shard leader (for the next round) and
	// send it back as part of CertificationResponse (AB-1719)
	if req.Leader == "" {
		return errors.New("leader ID must be assigned")
	}

	SI := v.getShardInfo(sysID, req.Shard)
	// todo: add SI based validations. However, first SI must be part of recovery? (AB-1718)
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID, req.Shard)
	if err != nil {
		return fmt.Errorf("reading last certified state: %w", err)
	}
	v.subscription.Subscribe(sysID, req.NodeIdentifier)
	if err = consensus.CheckBlockCertificationRequest(req, &latestUnicityCertificate.UC); err != nil {
		err = fmt.Errorf("invalid block certification request: %w", err)
		if se := v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); se != nil {
			err = errors.Join(err, fmt.Errorf("sending latest cert: %w", se))
		}
		return err
	}
	// check if consensus is already achieved
	if res := v.incomingRequests.IsConsensusReceived(sysID, req.Shard, pTrustBase); res != QuorumInProgress {
		return fmt.Errorf("rejecting stale block certification request: %s", res)
	}
	// store the new request and see if quorum is now achieved
	res, requests, err := v.incomingRequests.Add(req, pTrustBase)
	if err != nil {
		return fmt.Errorf("storing request: %w", err)
	}
	var reason consensus.CertReqReason
	switch res {
	case QuorumAchieved:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s reached consensus, new InputHash: %X", sysID, requests[0].InputRecord.Hash))
		reason = consensus.Quorum
	case QuorumNotPossible:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s consensus not possible, repeat UC", sysID))
		reason = consensus.QuorumNotPossible
	case QuorumInProgress:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s quorum not yet reached, but possible in the future", sysID))
		return nil
	}

	SI.Update(req)
	SI.Leader = "" // todo: select leader for the next round (AB-1719)
	if err = v.consensusManager.RequestCertification(ctx,
		consensus.IRChangeRequest{
			Partition: sysID,
			Shard:     req.Shard,
			Reason:    reason,
			Requests:  requests,
			Technical: certification.TechnicalRecord{
				Round:    SI.Round + 1,
				Epoch:    SI.Epoch,
				Leader:   SI.Leader,
				StatHash: SI.StatHash(crypto.SHA256),
				FeeHash:  SI.FeeHash(crypto.SHA256),
			},
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
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(cr.Partition, cr.Shard)
		v.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("Resetting request store for partition '%s'", cr.Partition))
	}()

	subscribed := v.subscription.Get(cr.Partition)
	v.log.DebugContext(ctx, fmt.Sprintf("sending unicity certificate to partition %s, IR Hash: %X, Block Hash: %X",
		cr.UC.UnicityTreeCertificate.SystemIdentifier, cr.UC.InputRecord.Hash, cr.UC.InputRecord.BlockHash))
	// send response to all registered nodes
	for _, node := range subscribed {
		if err := v.sendResponse(ctx, node, cr); err != nil {
			v.log.WarnContext(ctx, "sending certification result", logger.Error(err))
		}
		v.subscription.ResponseSent(cr.Partition, node)
	}
}

func (rcn *Node) getShardInfo(partition types.SystemID, shard types.ShardID) *shardInfo {
	key := partitionShard{partition: partition, shard: shard.Key()}
	if si, ok := rcn.shardInfo[key]; ok {
		return si
	}

	// todo: once ShardInfo recovery and epoch are implemented we expect all valid
	// SI to be in the registry! Here we currently use lazy initialization as we
	// do not have "SI registry init" in the beginning of the epoch yet

	si := &shardInfo{
		Fees:  make(map[string]uint64),
		Epoch: 0,
		// as we currently do not support epochs the data of the previous epoch
		// is "zero value constant", ie we can use precalculated "genesis values".
		// Once we implement epochs correct value must be recovered on node startup!
		PrevEpochStat: emptyStatRecSerialized,
		PrevEpochFees: nil,
	}
	rcn.shardInfo[key] = si
	return si
}

var emptyStatRecSerialized = (&certification.StatisticalRecord{}).Bytes()
