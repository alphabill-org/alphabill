package rootvalidator

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	proto "github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/rootchain"
	"github.com/alphabill-org/alphabill/internal/rootchain/store"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	PartitionNet interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	PartitionManager struct {
		store            StateStore
		net              PartitionNet
		timers           *timer.Timers
		partitionStore   *rootchain.PartitionStore
		incomingRequests rootchain.CertificationRequestStore
		hashAlgorithm    gocrypto.Hash // hash algorithm
		signer           crypto.Signer // private key of the root validator
		verifier         crypto.Verifier
	}
)

func (p *PartitionManager) Timers() *timer.Timers {
	return p.timers
}

func NewPartitionManager(signer crypto.Signer, n PartitionNet, stateStore StateStore, pStore *rootchain.PartitionStore) (*PartitionManager, error) {
	_, v, err := rootchain.GetPublicKeyAndVerifier(signer)
	if err != nil {
		return nil, errors.Wrap(err, "invalid rootvalidator private key")
	}
	if stateStore == nil {
		return nil, errors.New("root chain store is nil")
	}
	// Check state is initiated
	state, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	if state.LatestRound < 1 {
		return nil, errors.New("error state not initated")
	}

	timers := timer.NewTimers()

	return &PartitionManager{
		store:            stateStore,
		net:              n,
		timers:           timers,
		partitionStore:   pStore,
		incomingRequests: rootchain.NewCertificationRequestStore(),
		signer:           signer,
		verifier:         v}, nil
}

// OnTimeout handle t2 timers
func (p *PartitionManager) OnTimeout(timerId string) {
	logger.Debug("Handling timeout %s", timerId)
	p.timers.Restart(timerId)
}

func (p *PartitionManager) Receive() <-chan network.ReceivedMessage {
	return p.net.ReceivedChannel()
}

//OnPartitionMessage processes messages from partition validators
func (p *PartitionManager) OnPartitionMessage(msg *network.ReceivedMessage) {
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
		p.onBlockCertificationRequest(req)
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
}

// GetLatestUnicityCertificate returns last UC for system identifier
func (p *PartitionManager) GetLatestUnicityCertificate(id protocol.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	state, err := p.store.Get()
	if err != nil {
		return nil, err
	}
	luc, f := state.Certificates[id]
	if !f {
		return nil, errors.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
}

// OnBlockCertificationRequest handle certification requests from partition nodes.
// Partition nodes can only extend the stored/certified state
func (p *PartitionManager) onBlockCertificationRequest(req *certification.BlockCertificationRequest) {
	verifier, err := p.getPartitionVerifier(req.SystemIdentifier, req.NodeIdentifier)
	if err != nil {
		logger.Warning("invalid block certification request: %v", err)
		return
	}
	err = req.IsValid(verifier)
	if err != nil {
		logger.Warning("block certification request signature verification failed: %v", err)
		return
	}
	systemIdentifier := proto.SystemIdentifier(req.SystemIdentifier)
	latestUnicityCertificate, err := p.GetLatestUnicityCertificate(systemIdentifier)
	seal := latestUnicityCertificate.UnicitySeal
	if req.RootRoundNumber < seal.RootChainRoundNumber {
		// Older UC, return current.
		logger.Warning("old request: root validator round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
		// Send latestUnicityCertificate
		p.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	} else if req.RootRoundNumber > seal.RootChainRoundNumber {
		// should not happen, partition has newer UC
		logger.Warning("partition has never unicity certificate: root validator round number %v, partition node round number %v", seal.RootChainRoundNumber, req.RootRoundNumber)
		// send latestUnicityCertificate
		p.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	} else if !bytes.Equal(req.InputRecord.PreviousHash, latestUnicityCertificate.InputRecord.Hash) {
		// Extending of unknown State.
		logger.Warning("request extends unknown state: expected hash: %v, got: %v", seal.Hash, req.InputRecord.PreviousHash)
		// send latestUnicityCertificate
		p.sendResponse(req.NodeIdentifier, latestUnicityCertificate)
		return
	}
	if err := p.incomingRequests.Add(req); err != nil {
		logger.Warning("incoming request could not be stores: %v", err.Error())
		return
	}
	ir, consensusPossible := p.incomingRequests.IsConsensusReceived(systemIdentifier, p.partitionStore.NodeCount(systemIdentifier))
	if ir != nil || consensusPossible == false {
		logger.Info("Forward quorum proof for certification")
		// Forward all requests to next root chain leader (use channel?)
		// Todo: call atomic broadcast
		return
	}
}

func (p *PartitionManager) getPartitionVerifier(systemId []byte, nodeId string) (crypto.Verifier, error) {
	tb, err := p.partitionStore.GetTrustBase(proto.SystemIdentifier(systemId))
	if err != nil {
		return nil, err
	}
	ver, f := tb[nodeId]
	if !f {
		return nil, errors.Errorf("unknown node identifier %v", nodeId)
	}
	return ver, nil
}

func (p *PartitionManager) sendResponse(nodeId string, uc *certificates.UnicityCertificate) {
	peerID, err := peer.Decode(nodeId)
	if err != nil {
		logger.Warning("Invalid node identifier: '%s'", nodeId)
		return
	}
	logger.Info("Sending unicity certificate to '%s', IR Hash: %X, Block Hash: %X", nodeId, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
	err = p.net.Send(
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
