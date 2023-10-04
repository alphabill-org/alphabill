package rootchain

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/errgroup"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/debug"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
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
	}
)

// New creates a new instance of the root chain node
func New(
	p *network.Peer,
	pNet PartitionNet,
	ps partitions.PartitionConfiguration,
	cm consensus.Manager,
) (*Node, error) {
	if p == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}
	logger.Info("Starting root node. PeerId=%v; Addresses=%v; BuildInfo=%s", p.String(), p.MultiAddresses(), debug.ReadBuildInfo())
	node := &Node{
		peer:             p,
		partitions:       ps,
		incomingRequests: NewCertificationRequestBuffer(),
		subscription:     NewSubscriptions(),
		net:              pNet,
		consensusManager: cm,
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
			logger.Info("%v exiting root node main loop", v.peer.String())
			return ctx.Err()
		case msg, ok := <-v.net.ReceivedChannel():
			if !ok {
				logger.Warning("%v partition received channel closed, exiting root node main loop", v.peer.String())
				return fmt.Errorf("partition channel closed")
			}
			util.WriteTraceJsonLog(logger, fmt.Sprintf("received %T", msg), msg)
			switch mt := msg.(type) {
			case *certification.BlockCertificationRequest:
				v.onBlockCertificationRequest(ctx, mt)
			case *handshake.Handshake:
				v.onHandshake(ctx, mt)
			default:
				logger.Warning("%v message %s not supported.", v.peer.String(), msg)
			}
		}
	}
}

func (v *Node) sendResponse(ctx context.Context, nodeID string, uc *types.UnicityCertificate) error {
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", nodeID)
		return err
	}

	logger.Debug("%v sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		v.peer.String(), uc.UnicityTreeCertificate.SystemIdentifier, nodeID, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
	return v.net.Send(ctx, uc, peerID)
}

func (v *Node) onHandshake(ctx context.Context, req *handshake.Handshake) {
	if err := req.IsValid(); err != nil {
		logger.Warning("%v handshake error, invalid handshake request, %v", v.peer.String(), err)
		return
	}
	// process
	sysID := protocol.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		logger.Warning("%v handshake error, partition %X certificate read failed, %v", v.peer.String(), sysID.Bytes(), err)
		return
	}
	v.subscription.Subscribe(protocol.SystemIdentifier(req.SystemIdentifier), req.NodeIdentifier)
	if err = v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); err != nil {
		logger.Warning("%v handshake error, failed to send response, %v", v.peer.String(), err)
		return
	}
}

// onBlockCertificationRequest handles certification nodeRequest from partition nodes.
// Partition nodes can only extend the stored/certified state
func (v *Node) onBlockCertificationRequest(ctx context.Context, req *certification.BlockCertificationRequest) {
	if req.SystemIdentifier == nil {
		logger.Warning("%v invalid block certification request system id is nil, rejected", v.peer.String())
		return
	}
	sysID := protocol.SystemIdentifier(req.SystemIdentifier)
	_, pTrustBase, err := v.partitions.GetInfo(sysID)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected: %v",
			v.peer.String(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	if err = pTrustBase.Verify(req.NodeIdentifier, req); err != nil {
		logger.Warning("%v block certification request from %X node %v rejected: %v",
			v.peer.String(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected, failed to read last certified state %v",
			v.peer.String(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	v.subscription.Subscribe(sysID, req.NodeIdentifier)
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v invalid, %v",
			v.peer.String(), req.SystemIdentifier, req.NodeIdentifier, err)
		if err = v.sendResponse(ctx, req.NodeIdentifier, latestUnicityCertificate); err != nil {
			logger.Warning("%v send failed, %v", v.peer.String(), err)
		}
		return
	}
	// check if consensus is already achieved, then store, but it will not be used as proof
	if res := v.incomingRequests.IsConsensusReceived(sysID, pTrustBase); res != QuorumInProgress {
		// stale request buffer, but no need to add extra proof
		if _, _, err = v.incomingRequests.Add(req, pTrustBase); err != nil {
			logger.Warning("%v stale block certification request from %X node %v could not be stored, %v",
				v.peer.String(), err.Error())
		}
		return
	}
	// store new request and see if quorum is achieved
	res, proof, err := v.incomingRequests.Add(req, pTrustBase)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v could not be stored, %v",
			v.peer.String(), err.Error())
		return
	}
	switch res {
	case QuorumAchieved:
		logger.Debug("%v partition %X reached consensus, new InputHash: %X",
			v.peer.String(), sysID.Bytes(), proof[0].InputRecord.Hash)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.Quorum,
			Requests:         proof,
		}
	case QuorumNotPossible:
		logger.Debug("%v partition %X consensus not possible, repeat UC",
			v.peer.String(), sysID.Bytes())
		// add all nodeRequest to prove that no consensus is possible
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.QuorumNotPossible,
			Requests:         proof,
		}
	case QuorumInProgress:
		logger.Debug("%v partition %X quorum not yet reached, but possible in the future",
			v.peer.String(), sysID.Bytes())
	}
}

// handleConsensus - receives consensus results and delivers certificates to subscribers
func (v *Node) handleConsensus(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			logger.Info("%v exiting root consensus result handler", v.peer.String())
			return ctx.Err()
		case uc, ok := <-v.consensusManager.CertificationResult():
			if !ok {
				logger.Warning("%v consensus channel closed, exiting loop", v.peer.String())
				return fmt.Errorf("consensus channel closed")
			}
			v.onCertificationResult(ctx, uc)
		}
	}
}

func (v *Node) onCertificationResult(ctx context.Context, certificate *types.UnicityCertificate) {
	sysID := protocol.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new nodeRequest
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(sysID)
		logger.Trace("Resetting request store for partition '%X'", sysID.Bytes())
	}()
	subscribed := v.subscription.Get(sysID)
	// send response to all registered nodes
	for _, node := range subscribed {
		if err := v.sendResponse(ctx, node, certificate); err != nil {
			logger.Warning("%v send failed, %v", v.peer.String(), err)
			v.subscription.SubscriberError(sysID, node)
		}
	}
}
