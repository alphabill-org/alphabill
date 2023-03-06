package rootvalidator

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	proto "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	PartitionStore interface {
		Info(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	Node struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		peer             *network.Peer // p2p network host for partition
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
	if pNet == nil {
		return nil, fmt.Errorf("network is nil")
	}
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", host.LogID(), host.MultiAddresses())
	node := &Node{
		peer:             host,
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
			logger.Info("%v exiting root validator main loop", v.peer.LogID())
			return
		case msg, ok := <-v.net.ReceivedChannel():
			if !ok {
				logger.Warning("%v partition received channel closed, exiting root validator main loop", v.peer.LogID())
				return
			}
			if msg.Message == nil {
				logger.Warning("%v received partition message is nil", v.peer.LogID())
				return
			}
			switch msg.Protocol {
			case network.ProtocolBlockCertification:
				req, correctType := msg.Message.(*certification.BlockCertificationRequest)
				if !correctType {
					logger.Warning("%v type %T not supported", v.peer.LogID(), msg.Message)
					return
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Certification Request from %s", msg.From), req)
				v.onBlockCertificationRequest(req)
				break
			case network.ProtocolHandshake:
				req, correctType := msg.Message.(*handshake.Handshake)
				if !correctType {
					logger.Warning("%v type %T not supported", v.peer.LogID(), msg.Message)
					return
				}
				logger.Debug("%v handshake: system id %v, node %v", v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier)
				break
			default:
				logger.Warning("%v protocol %s not supported.", v.peer.LogID(), msg.Protocol)
				break
			}
		case uc, ok := <-v.consensusManager.CertificationResult():
			if !ok {
				logger.Warning("%v consensus channel closed, exiting loop", v.peer.LogID())
				return
			}
			v.onCertificationResult(&uc)
		}
	}
}

func (v *Node) sendResponse(nodeID string, uc *certificates.UnicityCertificate) {
	peerID, err := peer.Decode(nodeID)
	if err != nil {
		logger.Warning("%v invalid node identifier: '%s'", nodeID)
		return
	}
	logger.Info("%v sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		v.peer.LogID(), uc.UnicityTreeCertificate.SystemIdentifier, nodeID, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
	err = v.net.Send(
		network.OutputMessage{
			Protocol: network.ProtocolUnicityCertificates,
			Message:  uc,
		},
		[]peer.ID{peerID},
	)
	if err != nil {
		logger.Warning("%v failed to send unicity certificate: %v", v.peer.LogID(), err)
	}
}

// OnBlockCertificationRequest handle certification requests from partition nodes.
// Partition nodes can only extend the stored/certified state
func (v *Node) onBlockCertificationRequest(req *certification.BlockCertificationRequest) {
	sysID := proto.SystemIdentifier(req.SystemIdentifier)
	info, err := v.partitionStore.Info(sysID)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected: %v",
			v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	// find node verifier
	ver, found := info.TrustBase[req.NodeIdentifier]
	if !found {
		logger.Warning("%v block certification request from %X node %v rejected: unknown node",
			v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier)
		return
	}
	err = req.IsValid(ver)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected: signature verification failed",
			v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier)
		return
	}
	systemIdentifier := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.consensusManager.GetLatestUnicityCertificate(systemIdentifier)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v rejected, failed to read last certified state %v",
			v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		logger.Warning("%v block certification request from %X node %v invalid, %v",
			v.peer.LogID(), req.SystemIdentifier, req.NodeIdentifier, err)
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err = v.incomingRequests.Add(req); err != nil {
		logger.Warning("%v block certification request from %X node %v could not be stored, %v",
			v.peer.LogID(), err.Error())
		return
	}
	// There has to be at least one node in the partition, otherwise we could not have verified the request
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(info)
	// In case of quorum or no quorum possible forward the IR change request to consensus manager
	if ir != nil {
		logger.Info("%v partition %X reached consensus, new InputHash: %X",
			v.peer.LogID(), systemIdentifier.Bytes(), ir.Hash)
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.Quorum,
			Requests:         requests}
	} else if !consensusPossible {
		logger.Info("%v partition %X consensus not possible, repeat UC",
			v.peer.LogID(), systemIdentifier.Bytes())
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
		logger.Info("%v unable to send response to partition nodes: %v", v.peer.LogID(), err)
		return
	}
	// send response to all registered nodes
	for node := range info.TrustBase {
		v.sendResponse(node, certificate)
	}
}
