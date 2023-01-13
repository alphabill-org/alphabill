package rootvalidator

import (
	"context"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/monolithic"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
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
	ConsensusManager interface {
		RequestCertification() chan<- consensus.IRChangeRequest
		CertificationResult() <-chan certificates.UnicityCertificate
		Stop()
	}

	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	PartitionStoreRd interface {
		GetPartitionInfo(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	RootNodeConf struct {
		stateStore StateStore
	}
	Option func(c *RootNodeConf)

	ConsensusFn func(partition PartitionStoreRd, state StateStore) (ConsensusManager, error)

	Validator struct {
		ctx              context.Context
		ctxCancel        context.CancelFunc
		conf             *RootNodeConf
		partitionHost    *network.Peer // p2p network host for partition
		partitionStore   PartitionStoreRd
		incomingRequests *CertRequestBuffer
		net              PartitionNet
		consensusManager ConsensusManager
	}
)

func WithStateStore(store StateStore) Option {
	return func(c *RootNodeConf) {
		c.stateStore = store
	}
}

func MonolithicConsensus(selfId string, signer crypto.Signer) ConsensusFn {
	return func(partitionStore PartitionStoreRd, stateStore StateStore) (ConsensusManager, error) {
		return monolithic.NewMonolithicConsensusManager(selfId,
			partitionStore,
			stateStore,
			signer)
	}
}

func DistributedConsensus(rootHost *network.Peer, rootGenesis *genesis.GenesisRootRecord, rootNet *network.LibP2PNetwork, signer crypto.Signer) ConsensusFn {
	return func(partitionStore PartitionStoreRd, stateStore StateStore) (ConsensusManager, error) {
		return distributed.NewDistributedAbConsensusManager(
			rootHost,
			rootGenesis,
			partitionStore,
			stateStore,
			signer,
			rootNet)
	}
}

func initiateStateStore(stateStore distributed.StateStore, rg *genesis.RootGenesis) error {
	state, err := stateStore.Get()
	if err != nil {
		return err
	}
	// Init from genesis file is done only once
	if state.LatestRound > 0 {
		return nil
	}
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range rg.Partitions {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
	}
	// If not initiated, save genesis file to store
	if err := stateStore.Save(store.RootState{LatestRound: rg.GetRoundNumber(), Certificates: certs, LatestRootHash: rg.GetRoundHash()}); err != nil {
		return err
	}
	return nil
}

// NewRootValidatorNode creates a new instance of the root validator node
func NewRootValidatorNode(
	genesis *genesis.RootGenesis,
	prt *network.Peer,
	pNet PartitionNet,
	consensus ConsensusFn,
	opts ...Option,
) (*Validator, error) {
	if prt == nil {
		return nil, errors.New("partition listener is nil")
	}
	log.SetContext(log.KeyNodeID, prt.ID().String())
	if pNet == nil {
		return nil, errors.New("network is nil")
	}
	config := loadConf(opts)
	// Initiate state store
	err := initiateStateStore(config.stateStore, genesis)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate state store, %w", err)
	}
	// Initiate partition store
	partitionStore, err := partition_store.NewPartitionStoreFromGenesis(genesis.Partitions)
	if err != nil {
		return nil, fmt.Errorf("failed to initiate partition store: %w", err)
	}
	// create consensus manager
	consensusManager, err := consensus(partitionStore, config.stateStore)
	if err != nil {
		return nil, fmt.Errorf("failed to construct consensus manager, %w", err)
	}
	logger.Info("Starting root validator. PeerId=%v; Addresses=%v", prt.ID(), prt.MultiAddresses())
	node := &Validator{
		conf:             config,
		partitionHost:    prt,
		partitionStore:   partitionStore,
		incomingRequests: NewCertificationRequestBuffer(),
		net:              pNet,
		consensusManager: consensusManager,
	}
	node.ctx, node.ctxCancel = context.WithCancel(context.Background())
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
	logger.Info("Sending unicity certificate to partition %X node '%s', IR Hash: %X, Block Hash: %X",
		uc.UnicityTreeCertificate.SystemIdentifier, nodeId, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
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
	sysId := proto.SystemIdentifier(req.SystemIdentifier)
	info, err := v.partitionStore.GetPartitionInfo(sysId)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected, %w",
			req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	// find node verifier
	ver, found := info.TrustBase[req.NodeIdentifier]
	if !found {
		logger.Warning("Block certification request from %X node %v rejected, unknown node",
			req.SystemIdentifier, req.NodeIdentifier)
	}
	err = req.IsValid(ver)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected, signature verification failed",
			req.SystemIdentifier, req.NodeIdentifier)
		return
	}
	systemIdentifier := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := v.getLatestUnicityCertificate(systemIdentifier)
	if err != nil {
		logger.Warning("Block certification request from %X node %v rejected, failed to read last certified state %w",
			req.SystemIdentifier, req.NodeIdentifier, err)
		return
	}
	err = consensus.CheckBlockCertificationRequest(req, latestUnicityCertificate)
	if err != nil {
		logger.Warning("Block certification request from %X node %v invalid, %w", req.SystemIdentifier, req.NodeIdentifier, err)
		v.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err := v.incomingRequests.Add(req); err != nil {
		logger.Warning("Block certification request from %X node %v could not be stored, %w", err.Error())
		return
	}
	// There has to be at least one node in the partition, otherwise we could not have verified the request
	ir, consensusPossible := v.incomingRequests.IsConsensusReceived(info)
	// In case of quorum or no quorum possible forward the IR change request to consensus manager
	if ir != nil {
		logger.Info("Partition %X reached consensus, new InputHash: %X. ", systemIdentifier.Bytes(), ir.Hash)
		requests := v.incomingRequests.GetRequests(systemIdentifier)
		v.consensusManager.RequestCertification() <- consensus.IRChangeRequest{
			SystemIdentifier: systemIdentifier,
			Reason:           consensus.Quorum,
			IR:               ir,
			Requests:         requests}
	} else if !consensusPossible {
		logger.Info("Partition %X consensus not possible, repeat UC", systemIdentifier.Bytes())
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
			Reason:           consensus.QuorumNotPossible,
			IR:               luc.InputRecord,
			Requests:         requests}
	}
}

func (v *Validator) onCertificationResult(certificate *certificates.UnicityCertificate) {
	sysId := proto.SystemIdentifier(certificate.UnicityTreeCertificate.SystemIdentifier)
	// remember to clear the incoming buffer to accept new requests
	defer v.incomingRequests.Clear(sysId)

	info, err := v.partitionStore.GetPartitionInfo(sysId)
	if err != nil {
		logger.Info("Unable to send response to partition nodes: %v", err)
		return
	}
	// send response to all registered nodes
	for node := range info.TrustBase {
		v.sendResponse(node, certificate)
	}
}
