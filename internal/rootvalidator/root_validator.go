package rootvalidator

import (
	"context"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"

	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
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
	ConsensusManager interface {
		RequestCertification() chan<- consensus.IRChangeRequest
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

	PartitionStoreRd interface {
		GetInfo(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	RootNodeConf struct {
		stateStore StateStoreRd
	}
	Option func(c *RootNodeConf)

	Validator struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		conf             *RootNodeConf
		partitionHost    *network.Peer // p2p network host for partition
		partitionStore   PartitionStoreRd
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

// NewRootValidatorNode creates a new instance of the root validator node
func NewRootValidatorNode(
	consensus ConsensusManager,
	partitionStore PartitionStoreRd,
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
		incomingRequests: request_store.NewCertificationRequestStore(),
		net:              pNet,
		consensusManager: consensus,
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
	info, err := v.partitionStore.GetInfo(proto.SystemIdentifier(req.SystemIdentifier))
	if err != nil {
		logger.Warning("invalid block certification request: %v", err)
		return
	}
	verifier, f := info.TrustBase[req.NodeIdentifier]
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
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		logger.Warning("Invalid certification request: %w", err)
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err := v.incomingRequests.Add(req); err != nil {
		logger.Warning("incoming request could not be stores: %v", err.Error())
		return
	}
	// There has to be at least one node in the partition, otherwise we could not have verified the request
	nofNodes := len(info.TrustBase)
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(systemIdentifier, nofNodes)
	// In case of quorum or no quorum possible forward the IR change request to consensus manager
	if ir != nil {
		logger.Debug("Partition reached a consensus. SystemIdentifier: %X, InputHash: %X. ", systemIdentifier.Bytes(), ir.Hash)
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.Quorum,
			IR:               ir,
			Requests:         requests}
	} else if !consensusPossible {
		logger.Debug("Consensus not possible for partition %X.", systemIdentifier.Bytes())
		// Get last unicity certificate for the partition
		luc, err := v.getLatestUnicityCertificate(systemIdentifier)
		if err != nil {
			logger.Error("Cannot re-certify partition: SystemIdentifier: %X, error: %v", systemIdentifier.Bytes(), err.Error())
			return
		}
		// repeat IR for repeat UC
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.Quorum,
			IR:               luc.InputRecord,
			Requests:         requests}
	}
}

func (v *Validator) onCertificationResult(certificate *certificates.UnicityCertificate) {
	sysId := proto.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new requests
	defer v.incomingRequests.Clear(sysId)

	info, err := v.partitionStore.GetInfo(sysId)
	if err != nil {
		logger.Info("Unable to send response to partition nodes: %v", err)
		return
	}
	// send response to all registered nodes
	for node, _ := range info.TrustBase {
		v.sendResponse(node, certificate)
	}
}
