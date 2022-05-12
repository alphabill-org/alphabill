package rootchain

import (
	"context"
	"fmt"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/timer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/genesis"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/p1"
)

const t3TimerID = "t3timer"

var ErrPeerIsNil = errors.New("peer is nil")

const (
	defaultT3Timeout         = 900 * time.Millisecond
	defaultRequestChCapacity = 1000
)

type (
	RootChain struct {
		ctx        context.Context
		ctxCancel  context.CancelFunc
		peer       *network.Peer         // p2p network
		p1Protocol *p1.P1                // P1 protocol handler
		state      *State                // state of the root chain. keeps everything needed for consensus.
		timers     *timer.Timers         // keeps track of T2 and T3 timers
		requestsCh chan *p1.RequestEvent // incoming P1 requests channel
	}

	rootChainConf struct {
		t3Timeout         time.Duration
		requestChCapacity uint
	}

	Option func(c *rootChainConf)
)

func WithT3Timeout(timeout time.Duration) Option {
	return func(c *rootChainConf) {
		c.t3Timeout = timeout
	}
}

func WithRequestChCapacity(capacity uint) Option {
	return func(c *rootChainConf) {
		c.requestChCapacity = capacity
	}
}

// NewRootChain creates a new instance of the root chain.
func NewRootChain(peer *network.Peer, genesis *genesis.RootGenesis, signer crypto.Signer, opts ...Option) (*RootChain, error) {
	if peer == nil {
		return nil, ErrPeerIsNil
	}
	logger.Info("Starting Root Chain. PeerId=%v; Addresses=%v", peer.ID(), peer.MultiAddresses())
	s, err := NewStateFromGenesis(genesis, signer)
	if err != nil {
		return nil, err
	}

	conf := loadConf(opts)
	requestsCh := make(chan *p1.RequestEvent, conf.requestChCapacity)

	protocol, err := p1.NewRootChainCertificationProtocol(peer, requestsCh)
	if err != nil {
		return nil, err
	}

	timers := timer.NewTimers()
	timers.Start(t3TimerID, conf.t3Timeout)
	for _, p := range genesis.Partitions {
		for _, validator := range p.Nodes {
			duration := time.Duration(p.SystemDescriptionRecord.T2Timeout) * time.Millisecond
			timers.Start(string(validator.P1Request.SystemIdentifier), duration)
			break
		}
	}

	rc := &RootChain{
		peer:       peer,
		state:      s,
		p1Protocol: protocol,
		timers:     timers,
		requestsCh: requestsCh,
	}
	rc.ctx, rc.ctxCancel = context.WithCancel(context.Background())
	go rc.loop()
	return rc, nil
}

func (rc *RootChain) Close() {
	rc.timers.WaitClose()
	if rc.requestsCh != nil {
		close(rc.requestsCh)
	}
	if rc.p1Protocol != nil {
		rc.p1Protocol.Close()
	}
	rc.ctxCancel()
}

// loop handles messages from different goroutines.
func (rc *RootChain) loop() {
	for {
		select {
		case <-rc.ctx.Done():
			logger.Info("Exiting root chain main loop")
			return
		case e := <-rc.requestsCh:
			if e == nil {
				continue
			}
			util.WriteDebugJsonLog(logger, fmt.Sprintf("Handling Block Certification Request from peer %s", e.Req.NodeIdentifier), e.Req)
			rc.state.HandleInputRequestEvent(e)
		case nt := <-rc.timers.C:
			if nt == nil {
				continue
			}
			timerName := nt.Name()
			if timerName == t3TimerID {
				logger.Debug("Handling T3 timeout")
				identifiers, err := rc.state.CreateUnicityCertificates()
				if err != nil {
					logger.Warning("Round %v failed: %v", rc.state.roundNumber, err)
				}
				rc.timers.Restart(t3TimerID)

				for _, identifier := range identifiers {
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

func loadConf(opts []Option) *rootChainConf {
	conf := &rootChainConf{
		t3Timeout:         defaultT3Timeout,
		requestChCapacity: defaultRequestChCapacity,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(conf)
	}
	return conf
}
