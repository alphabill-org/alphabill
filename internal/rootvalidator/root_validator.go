package rootvalidator

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	proto "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
)

type (
	CertificationRequest struct {
		Requests []*certification.BlockCertificationRequest
	}
	ConsensusManager interface {
		RequestCertification() chan<- CertificationRequest
		CertificationResult() <-chan certificates.UnicityCertificate
		Start()
		Stop()
	}

	CertificationRequestStore interface {
		Add(request *certification.BlockCertificationRequest) error
		IsConsensusReceived(id proto.SystemIdentifier, nrOfNodes int) (*certificates.InputRecord, bool)
		GetRequests(id proto.SystemIdentifier) []*certification.BlockCertificationRequest
		Reset()
		Clear(id proto.SystemIdentifier)
	}

	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	StateStoreRd interface {
		Get() (store.RootState, error)
	}

	RootNodeConf struct {
		stateStore       StateStoreRd
		consensusManager ConsensusManager
	}
	Option func(c *RootNodeConf)

	Validator struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		conf             *RootNodeConf
		partitionHost    *network.Peer // p2p network host for partition
		partitionStore   *PartitionStore
		incomingRequests CertificationRequestStore
		net              PartitionNet
		consensusManager ConsensusManager
	}
)

func WithStateStore(store StateStoreRd) Option {
	return func(c *RootNodeConf) {
		c.stateStore = store
	}
}

func WithConsensusManager(consensus ConsensusManager) Option {
	return func(c *RootNodeConf) {
		c.consensusManager = consensus
	}
}

// NewRootValidatorNode creates a new instance of the root validator node
func NewRootValidatorNode(
	partitionStore *PartitionStore,
	prt *network.Peer,
	pNet PartitionNet,
	opts ...Option,
) (*Validator, error) {
	if prt == nil {
		return nil, errors.New("partition listener is nil")
	}
	log.SetContext(log.KeyNodeID, prt.ID().String())
	if pNet == nil {
		return nil, errors.New("network is nil")
	}
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", prt.ID(), prt.MultiAddresses())
	configuration := loadConf(opts)
	node := &Validator{
		conf:             configuration,
		partitionHost:    prt,
		partitionStore:   partitionStore,
		consensusManager: configuration.consensusManager,
	}
	node.ctx, node.ctxCancel = context.WithCancel(context.Background())
	// Start consensus manager
	node.consensusManager.Start()
	// Start receiving messages from partition nodes
	go node.loop()
	return node, nil
}

func (v *Validator) Close() {
	v.consensusManager.Stop()
	v.ctxCancel()
}

// loop handles messages from different goroutines.
func (v *Validator) loop() {
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
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling Block Certification Request from prt-listener %s", req.NodeIdentifier), req)
				logger.Debug("Handling Block Certification Request from prt-listener %s, IR hash %X, Block Hash %X", req.NodeIdentifier, req.InputRecord.Hash, req.InputRecord.BlockHash)
				v.onBlockCertificationRequest(req)
				break
			case network.ProtocolHandshake:
				req, correctType := msg.Message.(*handshake.Handshake)
				if !correctType {
					logger.Warning("Type %T not supported", msg.Message)
					return
				}
				util.WriteDebugJsonLog(logger, "Received handshake", req)
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

func loadConf(opts []Option) *RootNodeConf {
	conf := &RootNodeConf{
		stateStore: store.NewInMemStateStore(gocrypto.SHA256),
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}

func (v *Validator) getLatestUnicityCertificate(id protocol.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	state, err := v.conf.stateStore.Get()
	if err != nil {
		return nil, err
	}
	luc, f := state.Certificates[id]
	if !f {
		return nil, errors.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

func (v *Validator) sendResponse(nodeId string, uc *certificates.UnicityCertificate) {
	peerID, err := peer.Decode(nodeId)
	if err != nil {
		logger.Warning("Invalid node identifier: '%s'", nodeId)
		return
	}
	logger.Info("Sending unicity certificate to '%s', IR Hash: %X, Block Hash: %X", nodeId, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
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
func (v *Validator) onBlockCertificationRequest(req *certification.BlockCertificationRequest) {
	tb, err := v.partitionStore.GetTrustBase(proto.SystemIdentifier(req.SystemIdentifier))
	if err != nil {
		logger.Warning("invalid block certification request: %v", err)
		return
	}
	verifier, f := tb[req.NodeIdentifier]
	if !f {
		logger.Warning("block certification request from unknown node: %v", req.NodeIdentifier)
		return
	}
	err = req.IsValid(verifier)
	if err != nil {
		logger.Warning("block certification request signature verification failed: %v", err)
		return
	}
	systemIdentifier := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.getLatestUnicityCertificate(systemIdentifier)
	seal := latestUnicityCertificate.UnicitySeal
	if req.RootRoundNumber < seal.RootChainRoundNumber {
		// Older UC, return current.
		logger.Warning("old request: root validator round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
		// Send latestUnicityCertificate
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	} else if req.RootRoundNumber > seal.RootChainRoundNumber {
		// should not happen, partition has newer UC
		logger.Warning("partition has never unicity certificate: root validator round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
		// send latestUnicityCertificate
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	} else if !bytes.Equal(req.InputRecord.PreviousHash, latestUnicityCertificate.InputRecord.Hash) {
		// Extending of unknown State.
		logger.Warning("request extends unknown state: expected hash: %v, got: %v", seal.Hash, req.InputRecord.PreviousHash)
		// send latestUnicityCertificate
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err := v.incomingRequests.Add(req); err != nil {
		logger.Warning("incoming request could not be stores: %v", err.Error())
		return
	}
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(systemIdentifier, v.partitionStore.NodeCount(systemIdentifier))
	if ir != nil || consensusPossible == false {
		logger.Info("Forward quorum proof for certification")
		// Forward all requests to next root chain leader (use channel?)
		v.consensusManager.RequestCertification() <- CertificationRequest{}
		return
	}
}

func (v *Validator) onCertificationResult(certificate *certificates.UnicityCertificate) {
	sysId := proto.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new requests
	defer v.incomingRequests.Clear(sysId)

	nodes, err := v.partitionStore.GetNodes(sysId)
	if err != nil {
		logger.Info("Unable to send response to partition nodes: %v", err)
		return
	}
	for _, node := range nodes {
		v.sendResponse(node, certificate)
	}
}
