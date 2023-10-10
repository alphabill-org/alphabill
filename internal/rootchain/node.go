package rootchain

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

type (
	PartitionNet interface {
		Send(ctx context.Context, msg any, receivers ...peer.ID) error
		ReceivedChannel() <-chan any
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
	}
)

// New creates a new instance of the root chain node
func New(
	p *network.Peer,
	pNet PartitionNet,
	ps partitions.PartitionConfiguration,
	cm consensus.Manager,
	log *slog.Logger,
) (*Node, error) {
	if p == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}
	log.Info(fmt.Sprintf("Starting root node. Addresses=%v; BuildInfo=%s", p.MultiAddresses(), debug.ReadBuildInfo()))
	node := &Node{
		peer:             p,
		partitions:       ps,
		incomingRequests: NewCertificationRequestBuffer(),
		subscription:     NewSubscriptions(),
		net:              pNet,
		consensusManager: cm,
		log:              log,
	}
	return node, nil
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
	sysID := req.SystemIdentifier.ToSystemID32()
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		return fmt.Errorf("reading partition %08X certificate: %w", sysID, err)
	}
	v.subscription.Subscribe(sysID, req.NodeIdentifier)
	if err = v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); err != nil {
		return fmt.Errorf("failed to send response: %w", err)
	}
	return nil
}

// onBlockCertificationRequest handles certification nodeRequest from partition nodes.
// Partition nodes can only extend the stored/certified state
func (v *Node) onBlockCertificationRequest(ctx context.Context, req *certification.BlockCertificationRequest) error {
	if req.SystemIdentifier == nil {
		return fmt.Errorf("invalid block certification request, system id is nil")
	}
	sysID := req.SystemIdentifier.ToSystemID32()
	_, pTrustBase, err := v.partitions.GetInfo(sysID)
	if err != nil {
		return fmt.Errorf("reading partition info: %w", err)
	}
	if err = pTrustBase.Verify(req.NodeIdentifier, req); err != nil {
		return fmt.Errorf("%X node %v rejected: %w", req.SystemIdentifier, req.NodeIdentifier, err)
	}
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		return fmt.Errorf("reading last certified state: %w", err)
	}
	v.subscription.Subscribe(sysID, req.NodeIdentifier)
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		err = fmt.Errorf("invalid block certification request: %w", err)
		if se := v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); se != nil {
			err = errors.Join(err, fmt.Errorf("sending latest cert: %w", se))
		}
		return err
	}
	// check if consensus is already achieved, then store, but it will not be used as proof
	if res := v.incomingRequests.IsConsensusReceived(sysID, pTrustBase); res != QuorumInProgress {
		// stale request buffer, but no need to add extra proof
		if _, _, err = v.incomingRequests.Add(req, pTrustBase); err != nil {
			return fmt.Errorf("stale block certification request, could not be stored: %w", err)
		}
		return nil
	}
	// store new request and see if quorum is achieved
	res, proof, err := v.incomingRequests.Add(req, pTrustBase)
	if err != nil {
		return fmt.Errorf("storing request: %w", err)
	}
	switch res {
	case QuorumAchieved:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %08X reached consensus, new InputHash: %X", sysID, proof[0].InputRecord.Hash))
		select {
		case v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.Quorum,
			Requests:         proof,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	case QuorumNotPossible:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %08X consensus not possible, repeat UC", sysID))
		// add all nodeRequest to prove that no consensus is possible
		select {
		case v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.QuorumNotPossible,
			Requests:         proof,
		}:
		case <-ctx.Done():
			return ctx.Err()
		}
	case QuorumInProgress:
		v.log.DebugContext(ctx, fmt.Sprintf("partition %08X quorum not yet reached, but possible in the future", sysID))
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
	sysID := certificate.UnicityTreeCertificate.SystemIdentifier.ToSystemID32()
	// remember to clear the incoming buffer to accept new nodeRequest
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(sysID)
		v.log.LogAttrs(ctx, logger.LevelTrace, fmt.Sprintf("Resetting request store for partition '%08X'", sysID))
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
