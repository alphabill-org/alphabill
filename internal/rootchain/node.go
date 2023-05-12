package rootchain

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network"
	proto "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"golang.org/x/sync/errgroup"
)

type (
	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
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
	logger.Info("Starting root node. PeerId=%v; Addresses=%v", p.String(), p.MultiAddresses())
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
			if msg.Message == nil {
				logger.Warning("%v received partition message is nil", v.peer.String())
			}
			switch msg.Protocol {
			case network.ProtocolBlockCertification:
				req, correctType := msg.Message.(*certification.BlockCertificationRequest)
				if !correctType {
					logger.Warning("%v type %T not supported", v.peer.String(), msg.Message)
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Certification Request from %s", msg.From), req)
				v.onBlockCertificationRequest(req)
			case network.ProtocolHandshake:
				req, correctType := msg.Message.(*handshake.Handshake)
				if !correctType {
					logger.Warning("%v type %T not supported", v.peer.String(), msg.Message)
				}
				util.WriteTraceJsonLog(logger, fmt.Sprintf("Handshake from %s", msg.From), req)
				v.onHandshake(req)
			default:
				logger.Warning("%v protocol %s not supported.", v.peer.String(), msg.Protocol)
			}
		}
	}
}

func (v *Node) sendResponse(nodeID string, uc *certificates.UnicityCertificate) error {
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", nodeID)
		return err
	}
	logger.Debug("%v sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		v.peer.String(), uc.UnicityTreeCertificate.SystemIdentifier, nodeID, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
	return v.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolUnicityCertificates,
			Message:  uc,
		},
		[]peer.ID{peerID},
	)
}

func (v *Node) onHandshake(req *handshake.Handshake) {
	if err := req.IsValid(); err != nil {
		logger.Warning("%v handshake error, invalid handshake request, %v", v.peer.String(), err)
		return
	}
	// process
	sysID := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(sysID)
	if err != nil {
		logger.Warning("%v handshake error, partition %X certificate read failed, %v", v.peer.String(), sysID.Bytes(), err)
		return
	}
	v.subscription.Subscribe(proto.SystemIdentifier(req.SystemIdentifier), req.NodeIdentifier)
	if err = v.sendResponse(req.NodeIdentifier, latestUnicityCertificate); err != nil {
		logger.Warning("%v handshake error, failed to send response, %v", v.peer.String(), err)
		return
	}
}

// OnBlockCertificationRequest handle certification requests from partition nodes.
// Partition nodes can only extend the stored/certified state
func (v *Node) onBlockCertificationRequest(req *certification.BlockCertificationRequest) {
	if req.SystemIdentifier == nil {
		logger.Warning("%v invalid block certification request system id is nil, rejected", v.peer.String())
		return
	}
	sysID := proto.SystemIdentifier(req.SystemIdentifier)
	_, ver, err := v.partitions.GetInfo(sysID)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected: %v",
			v.peer.String(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	if err = ver.Verify(req.NodeIdentifier, req); err != nil {
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
		if err = v.sendResponse(req.NodeIdentifier, latestUnicityCertificate); err != nil {
			logger.Warning("%v send failed, %v", v.peer.String(), err)
		}
		return
	}
	// check if consensus is already achieved
	if ir, _ := v.incomingRequests.IsConsensusReceived(sysID, ver); ir != nil {
		// stale request buffer, but no need to add extra proof
		if err = v.incomingRequests.Add(req); err != nil {
			logger.Warning("%v stale block certification request from %X node %v could not be stored, %v",
				v.peer.String(), err.Error())
		}
		return
	}

	if err = v.incomingRequests.Add(req); err != nil {
		logger.Warning("%v block certification request from %X node %v could not be stored, %v",
			v.peer.String(), err.Error())
		return
	}
	// There has to be at least one node in the partition, otherwise we could not have verified the request
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(sysID, ver)
	// In case of quorum or no quorum possible forward the IR change request to consensus manager
	if ir != nil {
		logger.Debug("%v partition %X reached consensus, new InputHash: %X",
			v.peer.String(), sysID.Bytes(), ir.Hash)
		requests := v.incomingRequests.GetRequests(sysID)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.Quorum,
			Requests:         requests,
		}
		return
	}
	if !consensusPossible {
		logger.Debug("%v partition %X consensus not possible, repeat UC",
			v.peer.String(), sysID.Bytes())
		// add all requests to prove that no consensus is possible
		requests := v.incomingRequests.GetRequests(sysID)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: sysID,
			Reason:           consensus.QuorumNotPossible,
			Requests:         requests,
		}
		return
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
				return fmt.Errorf("consenus channel closed")
			}
			v.onCertificationResult(uc)
		}
	}
}

func (v *Node) onCertificationResult(certificate *certificates.UnicityCertificate) {
	sysID := proto.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new requests
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer func() {
		v.incomingRequests.Clear(sysID)
		logger.Trace("Resetting request store for partition '%X'", sysID.Bytes())
	}()
	subscribed := v.subscription.Get(sysID)
	// send response to all registered nodes
	for _, node := range subscribed {
		if err := v.sendResponse(node, certificate); err != nil {
			logger.Warning("%v send failed, %v", v.peer.String(), err)
			v.subscription.SubscriberError(sysID, node)
		}
	}
}
