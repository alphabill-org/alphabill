package rootchain

import (
	"context"
	"time"

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
		p1         *p1.P1                // P1 protocol handler
		state      *state                // state of the root chain. keeps everything needed for consensus.
		timers     *timers               // keeps track of T2 and T3 timers
		requestsCh chan *p1.RequestEvent // incoming P1 requests channel
	}

	rootChainConf struct {
		t2Timeout         time.Duration
		requestChCapacity uint
	}

	Option func(c *rootChainConf)
)

func WithT3Timeout(timeout time.Duration) Option {
	return func(c *rootChainConf) {
		c.t2Timeout = timeout
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
	s, err := newStateFromGenesis(genesis, signer)
	if err != nil {
		return nil, err
	}

	conf := loadConf(opts)
	requestsCh := make(chan *p1.RequestEvent, conf.requestChCapacity)

	p1, err := p1.New(peer, requestsCh)
	if err != nil {
		return nil, err
	}

	timers := NewTimers()
	timers.Start(t3TimerID, conf.t2Timeout)
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
		p1:         p1,
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
	if rc.p1 != nil {
		rc.p1.Close()
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
			WriteDebugJsonLog(logger, "Handling P1 request", e.Req)
			rc.state.handleInputRequestEvent(e)
		case nt := <-rc.timers.C:
			if nt == nil {
				continue
			}
			if nt.name == t3TimerID {
				logger.Debug("Handling T3 timeout")
				identifiers, err := rc.state.createUnicityCertificates()
				if err != nil {
					logger.Warning("round %v failed: %v", rc.state.roundNumber, err)
				}
				rc.timers.Restart(t3TimerID)

				for _, identifier := range identifiers {
					logger.Debug("Restarting T2 timer: %X", []byte(identifier))
					rc.timers.Restart(identifier)
				}
			} else {
				logger.Debug("Handling T2 timeout with ID '%X'", []byte(nt.name))
				rc.state.copyOldInputRecords(nt.name)
				rc.timers.Restart(nt.name)
			}
		}
	}
}

func loadConf(opts []Option) *rootChainConf {
	conf := &rootChainConf{
		t2Timeout:         defaultT3Timeout,
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
