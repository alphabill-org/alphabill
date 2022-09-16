package rootchain

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"

	log "github.com/alphabill-org/alphabill/internal/logger"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p-core/peer"
)

const (
	defaultT3Timeout = 900 * time.Millisecond
	t3TimerID        = "t3timer"
)

type (
	// Net provides an interface for sending messages to and receiving messages from other nodes in the network.
	Net interface {
		Send(msg network.OutputMessage, receivers []peer.ID) error
		ReceivedChannel() <-chan network.ReceivedMessage
	}

	RootChain struct {
		ctx       context.Context
		ctxCancel context.CancelFunc
		net       Net
		peer      *network.Peer // p2p network
		state     *State        // state of the root chain. keeps everything needed for consensus.
		timers    *timer.Timers // keeps track of T2 and T3 timers
	}

	rootChainConf struct {
		t3Timeout time.Duration
	}

	Option func(c *rootChainConf)
)

func WithT3Timeout(timeout time.Duration) Option {
	return func(c *rootChainConf) {
		c.t3Timeout = timeout
	}
}

// NewRootChain creates a new instance of the root chain.
func NewRootChain(peer *network.Peer, genesis *genesis.RootGenesis, signer crypto.Signer, net Net, opts ...Option) (*RootChain, error) {
	if peer == nil {
		return nil, errors.New("peer is nil")
	}
	selfId := peer.ID().String()
	log.SetContext(log.KeyNodeID, selfId)
	if net == nil {
		return nil, errors.New("network is nil")
	}
	// todo root genesis: this will become obsolete when dynamic configuration is implemented??
	// Locate local node from genesis info
	nodeInfo := genesis.Root.FindPubKeyById(peer.ID().String())
	if nodeInfo == nil {
		logger.Info("Root node %v info not in genesis file", peer.ID().String())
		return nil, errors.New("invalid root validator encode key")
	}
	ver, err := signer.Verifier()
	if err != nil {
		return nil, errors.Wrap(err, "invalid root validator sign key, cannot start")
	}
	signPubKeyBytes, err := ver.MarshalPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "invalid root validator sign key, cannot start")
	}
	// verify that the same public key is present in the genesis file
	if !bytes.Equal(signPubKeyBytes, nodeInfo.SigningPublicKey) {
		return nil, errors.Errorf("invalid root validator sign key, expected %X, got %X", signPubKeyBytes, nodeInfo.SigningPublicKey)
	}

	logger.Info("Starting Root Chain. PeerId=%v; Addresses=%v", peer.ID(), peer.MultiAddresses())
	s, err := NewStateFromGenesis(genesis, selfId, signer)
	if err != nil {
		return nil, err
	}

	conf := loadConf(opts)

	if err != nil {
		return nil, err
	}

	timers := timer.NewTimers()
	timers.Start(t3TimerID, conf.t3Timeout)
	for _, p := range genesis.Partitions {
		for _, validator := range p.Nodes {
			duration := time.Duration(p.SystemDescriptionRecord.T2Timeout) * time.Millisecond
			timers.Start(string(validator.BlockCertificationRequest.SystemIdentifier), duration)
			break
		}
	}

	if err != nil {
		return nil, err
	}

	rc := &RootChain{
		net:    net,
		peer:   peer,
		state:  s,
		timers: timers,
	}
	rc.ctx, rc.ctxCancel = context.WithCancel(context.Background())
	go rc.loop()
	return rc, nil
}

func (rc *RootChain) Close() {
	rc.timers.WaitClose()
	rc.ctxCancel()
}

// loop handles messages from different goroutines.
func (rc *RootChain) loop() {
	for {
		select {
		case <-rc.ctx.Done():
			logger.Info("Exiting root chain main loop")
			return
		case m, ok := <-rc.net.ReceivedChannel():
			if !ok {
				logger.Warning("Received channel closed, exiting root chain main loop")
				return
			}
			if m.Message == nil {
				logger.Warning("Received network message is nil")
				continue
			}
			switch m.Protocol {
			case network.ProtocolBlockCertification:
				req, correctType := m.Message.(*certification.BlockCertificationRequest)
				if !correctType {
					logger.Warning("Type %T not supported", m.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling Block Certification Request from peer %s", req.NodeIdentifier), req)
				logger.Debug("Handling Block Certification Request from peer %s, IR hash %X, Block Hash %X", req.NodeIdentifier, req.InputRecord.Hash, req.InputRecord.BlockHash)
				uc, err := rc.state.HandleBlockCertificationRequest(req)
				if err != nil {
					logger.Warning("invalid block certification request: %v", err)
				}
				// state.HandleBlockCertificationRequest function may return both error and uc (e.g. if partition node
				// does not have the latest unicity certificate)
				if uc != nil {
					peerID, err := peer.Decode(req.NodeIdentifier)
					if err != nil {
						logger.Warning("Invalid node identifier: '%s'", req.NodeIdentifier)
						continue
					}
					logger.Info("Sending unicity certificate to '%s', IR Hash: %X, Block Hash: %X", req.NodeIdentifier, uc.InputRecord.Hash, uc.InputRecord.BlockHash)
					err = rc.net.Send(
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
			case network.ProtocolHandshake:
				req, correctType := m.Message.(*handshake.Handshake)
				if !correctType {
					logger.Warning("Type %T not supported", m.Message)
					continue
				}
				util.WriteDebugJsonLog(logger, "Received handshake", req)
			default:
				logger.Warning("Protocol %s not supported.", m.Protocol)
			}
		case nt := <-rc.timers.C:
			if nt == nil {
				continue
			}
			timerName := nt.Name()
			if timerName == t3TimerID {
				logger.Debug("Handling T3 timeout")
				partitionIdentifiers, err := rc.state.CreateUnicityCertificates()
				if err != nil {
					logger.Warning("Round %v failed: %v", rc.state.roundNumber, err)
				}
				rc.sendUC(partitionIdentifiers)
				rc.timers.Restart(t3TimerID)
				for _, identifier := range partitionIdentifiers {
					logger.Debug("Restarting T2 timer: %X", []byte(identifier))
					rc.timers.Restart(identifier)
				}
			} else {
				logger.Debug("Handling T2 timeout with a name '%X'", []byte(timerName))
				rc.state.CopyOldInputRecords(timerName)
				rc.timers.Restart(timerName)
			}
		}
	}
}

func (rc *RootChain) sendUC(identifiers []string) {
	for _, identifier := range identifiers {
		uc := rc.state.latestUnicityCertificates.get(identifier)
		if uc == nil {
			// we don't have uc; continue with the next identifier
			logger.Warning("Latest UC does not exist for partition: %v", identifier)
			continue
		}

		partition := rc.state.partitionStore.get(identifier)
		if partition == nil {
			// we don't have the partition information; continue with the next identifier
			logger.Warning("Partition information does not exist for partition: %v", identifier)
			continue
		}
		var ids []peer.ID
		for _, v := range partition.Validators {
			nodeID, err := peer.Decode(v.NodeIdentifier)
			if err != nil {
				logger.Warning("Invalid validator ID %v: %v", v.NodeIdentifier, err)
				continue
			}
			ids = append(ids, nodeID)
		}

		err := rc.net.Send(
			network.OutputMessage{
				Protocol: network.ProtocolUnicityCertificates,
				Message:  uc,
			},
			ids,
		)
		if err != nil {
			logger.Warning("Failed to send unicity certificates to all or some peers in the network: %v", err)
		}
	}
}

func loadConf(opts []Option) *rootChainConf {
	conf := &rootChainConf{
		t3Timeout: defaultT3Timeout,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}
