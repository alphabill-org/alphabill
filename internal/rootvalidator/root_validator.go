package rootvalidator

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	proto "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	StateStore interface {
		Save(state *store.RootState) error
		Get() (*store.RootState, error)
	}

	PartitionStore interface {
		Info(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	Node struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		partitionHost    *network.Peer // p2p network host for partition
		partitionStore   PartitionStore
		incomingRequests *CertRequestBuffer
		net              PartitionNet
		consensusManager consensus.Manager
	}
)

// NewRootValidatorNode creates a new instance of the root validator node
func NewRootValidatorNode(
	host *network.Peer,
	pNet PartitionNet,
	ps PartitionStore,
	cm consensus.Manager,
) (*Node, error) {
	if host == nil {
		return nil, fmt.Errorf("partition listener is nil")
	}
	log.SetContext(log.KeyNodeID, host.ID().String())
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", host.ID(), host.MultiAddresses())
	node := &Node{
		partitionHost:    host,
		partitionStore:   ps,
		incomingRequests: NewCertificationRequestBuffer(),
		net:              pNet,
		consensusManager: cm,
	}
	node.ctx, node.ctxCancel = context.WithCancel(context.Background())
	// Start receiving messages from partition nodes
	go node.loop()
	return node, nil
}

func (v *Node) Close() {
	v.consensusManager.Stop()
	v.ctxCancel()
}

// loop handles messages from different goroutines.
func (v *Node) loop() {
	for {
		select {
		case <-v.ctx.Done():
			logger.Info("Exiting root validator main loop")
			return
		case msg, ok := <-v.net.ReceivedChannel():
			if !ok {
				logger.Warning("Partition received channel closed, exiting root validator main loop")
				return
			}
			if msg.Message == nil {
				logger.Warning("Received partition message is nil")
				return
			}
			switch msg.Protocol {
			case network.ProtocolBlockCertification:
				req, correctType := msg.Message.(*certification.BlockCertificationRequest)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					return
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Certification Request from %s", msg.From), req)
				v.onBlockCertificationRequest(req)
				break
			case network.ProtocolHandshake:
				req, correctType := msg.Message.(*handshake.Handshake)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					return
				}
				logger.Debug("Handshake: system id %v, node %v", req.SystemIdentifier, req.NodeIdentifier)
				break
			default:
				logger.Warning("Protocol %s not supported.", msg.Protocol)
				break
			}
		case uc, ok := <-v.consensusManager.CertificationResult():
			if !ok {
				logger.Warning("Consensus channel closed, exiting loop")
				return
			}
			v.onCertificationResult(&uc)
		}
	}
}

func (v *Node) sendResponse(nodeID string, uc *certificates.UnicityCertificate) {
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		logger.Warning("Invalid node identifier: '%s'", nodeID)
		return
	}
	logger.Info("Sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		uc.UnicityTreeCertificate.SystemIdentifier, nodeID, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
	err = v.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolUnicityCertificates,
			Message:  uc,
		},
		[]peer.ID{peerID},
	)
	if err != nil {
		logger.Warning("Failed to send unicity certificate: %v", err)
	}
}

// OnBlockCertificationRequest handle certification requests from partition nodes.
// Partition nodes can only extend the stored/certified state
func (v *Node) onBlockCertificationRequest(req *certification.BlockCertificationRequest) {
	sysID := proto.SystemIdentifier(req.SystemIdentifier)
	info, err := v.partitionStore.Info(sysID)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected: %v",
			req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	// find node verifier
	ver, found := info.TrustBase[req.NodeIdentifier]
	if !found {
		logger.Warning("Block certification request from %X node %v rejected: unknown node",
			req.SystemIdentifier, req.NodeIdentifier)
		return
	}
	err = req.IsValid(ver)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected: signature verification failed",
			req.SystemIdentifier, req.NodeIdentifier)
		return
	}
	systemIdentifier := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(systemIdentifier)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected, failed to read last certified state %v",
			req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		logger.Warning("Block certification request from %X node %v invalid, %v", req.SystemIdentifier, req.NodeIdentifier, err)
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err := v.incomingRequests.Add(req); err != nil {
		logger.Warning("Block certification request from %X node %v could not be stored, %v", err.Error())
		return
	}
	// There has to be at least one node in the partition, otherwise we could not have verified the request
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(info)
	// In case of quorum or no quorum possible forward the IR change request to consensus manager
	if ir != nil {
		logger.Info("Partition %X reached consensus, new InputHash: %X", systemIdentifier.Bytes(), ir.Hash)
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.Quorum,
			Requests:         requests}
	} else if !consensusPossible {
		logger.Info("Partition %X consensus not possible, repeat UC", systemIdentifier.Bytes())
		// add all requests to prove that no consensus is possible
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.QuorumNotPossible,
			Requests:         requests}
	}
}

func (v *Node) onCertificationResult(certificate *certificates.UnicityCertificate) {
	sysID := proto.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new requests
	// NB! this will try and reset the store also in the case when system id is unknown, but this is fine
	defer v.incomingRequests.Clear(sysID)

	info, err := v.partitionStore.Info(sysID)
	if err != nil {
		logger.Info("Unable to send response to partition nodes: %v", err)
		return
	}
	// send response to all registered nodes
	for node := range info.TrustBase {
		v.sendResponse(node, certificate)
	}
}
