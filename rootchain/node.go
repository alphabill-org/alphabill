package rootchain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/types"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/errgroup"
)

type (
	PartitionNet interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any
	}

	Observability interface {
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
	}

	MsgVerification interface {
		IsValid(v crypto.Verifier) error
	}

	Node struct {
		peer             *network.Peer // p2p network host for partition
		partitions       partitions.PartitionConfiguration
		incomingRequests *CertRequestBuffer
		subscription     *Subscriptions
		net              PartitionNet
		consensusManager consensus.Manager
		log              *slog.Logger

		bcrCount metric.Int64Counter // Block Certification Request count
	}
)

// New creates a new instance of the root chain node
func New(
	p *network.Peer,
	pNet PartitionNet,
	ps partitions.PartitionConfiguration,
	cm consensus.Manager,
	observe Observability,
) (*Node, error) {
	if p == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}
	observe.Logger().Info(fmt.Sprintf("Starting root node. Addresses=%v; BuildInfo=%s", p.MultiAddresses(), debug.ReadBuildInfo()))
	node := &Node{
		peer:             p,
		partitions:       ps,
		incomingRequests: NewCertificationRequestBuffer(),
		subscription:     NewSubscriptions(),
		net:              pNet,
		consensusManager: cm,
		log:              observe.Logger(),
	}
	if err := node.initMetrics(observe); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}
	return node, nil
}

func (n *Node) initMetrics(observe Observability) (err error) {
	m := observe.Meter("rootchain", metric.WithInstrumentationAttributes(attribute.String("node_id", string(n.peer.ID()))))

	n.bcrCount, err = m.Int64Counter("block.cert.req", metric.WithDescription("Number of Block Certification Requests processed"))
	if err != nil {
		return fmt.Errorf("creating Block Certification Requests counter: %w", err)
	}

	return nil
}

func (v *Node) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	// Run root consensus algorithm
	g.Go(func() error { return v.consensusManager.Run(gctx) })
	// Start receiving messages from partition nodes
	g.Go(func() error { return v.loop(gctx) })
	// Start handling certification responses
	g.Go(func() error { return v.handleConsensus(gctx) })
	return g.Wait()
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
				v.log.WarnContext(ctx, "message %T not supported.", msg)
			}
		}
	}
}

func (v *Node) sendResponse(ctx context.Context, nodeID string, uc *types.UnicityCertificate) error {
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		return fmt.Errorf("invalid receiver id: %w", err)
	}

	v.log.DebugContext(ctx, fmt.Sprintf("sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		uc.UnicityTreeCertificate.SystemIdentifier, nodeID, uc.InputRecord.Hash, uc.InputRecord.BlockHash))
	return v.net.Send(ctx, uc, peerID)
}

func (v *Node) onHandshake(ctx context.Context, req *handshake.Handshake) error {
	if err := req.IsValid(); err != nil {
		return fmt.Errorf("invalid handshake request: %w", err)
	}
	// process
	// ignore error here, as validate already checks system identifier length, conversion cannot fail
	sysID, _ := req.SystemIdentifier.Id32()
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		return fmt.Errorf("reading partition %s certificate: %w", sysID, err)
	}
	v.subscription.Subscribe(sysID, req.NodeIdentifier)
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
	sysID32, err := req.SystemIdentifier.Id32()
	if err != nil {
		v.bcrCount.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.String("status", "err.sysid"))))
		return fmt.Errorf("request contains invalid system identifier %X: %w", req.SystemIdentifier, err)
	}
	defer func() {
		status := "ok"
		if rErr != nil {
			status = "err"
		}
		v.bcrCount.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.String("status", status), attribute.Int("partition", int(sysID32)))))
	}()

	_, pTrustBase, err := v.partitions.GetInfo(sysID32)
	if err != nil {
		return fmt.Errorf("reading partition info: %w", err)
	}
	if err = pTrustBase.Verify(req.NodeIdentifier, req); err != nil {
		return fmt.Errorf("%X node %v rejected: %w", req.SystemIdentifier, req.NodeIdentifier, err)
	}
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID32)
	if err != nil {
		return fmt.Errorf("reading last certified state: %w", err)
	}
	v.subscription.Subscribe(sysID32, req.NodeIdentifier)
	if err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate); err != nil {
		err = fmt.Errorf("invalid block certification request: %w", err)
		if se := v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); se != nil {
			err = errors.Join(err, fmt.Errorf("sending latest cert: %w", se))
		}
		return err
	}
	// check if consensus is already achieved, then store, but it will not be used as proof
	if res := v.incomingRequests.IsConsensusReceived(sysID32, pTrustBase); res != QuorumInProgress {
		// stale request buffer, but no need to add extra proof
		if _, _, err = v.incomingRequests.Add(sysID32, req, pTrustBase); err != nil {
			return fmt.Errorf("stale block certification request, could not be stored: %w", err)
		}
		return nil
	}
	// store new request and see if quorum is achieved
	res, proof, err := v.incomingRequests.Add(sysID32, req, pTrustBase)
	if err != nil {
		return fmt.Errorf("storing request: %w", err)
	}
	switch res {
	case QuorumAchieved:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s reached consensus, new InputHash: %X", sysID32, proof[0].InputRecord.Hash))
		select {
		case v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID32,
			Reason:           consensus.Quorum,
			Requests:         proof,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	case QuorumNotPossible:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s consensus not possible, repeat UC", sysID32))
		// add all nodeRequest to prove that no consensus is possible
		select {
		case v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID32,
			Reason:           consensus.QuorumNotPossible,
			Requests:         proof,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	case QuorumInProgress:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %s quorum not yet reached, but possible in the future", sysID32))
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

func (v *Node) onCertificationResult(ctx context.Context, certificate *types.UnicityCertificate) {
	sysID, err := certificate.UnicityTreeCertificate.SystemIdentifier.Id32()
	if err != nil {
		v.log.WarnContext(ctx, "failed to send certification result", logger.Error(err))
		return
	}
	// remember to clear the incoming buffer to accept new nodeRequest
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(sysID)
		v.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("Resetting request store for partition '%s'", sysID))
	}()
	subscribed := v.subscription.Get(sysID)
	// send response to all registered nodes
	for _, node := range subscribed {
		if err := v.sendResponse(ctx, node, certificate); err != nil {
			v.log.WarnContext(ctx, "sending certification result", logger.Error(err))
			v.subscription.SubscriberError(sysID, node)
		}
	}
}
