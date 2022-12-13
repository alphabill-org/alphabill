package monolithic

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	log "github.com/alphabill-org/alphabill/internal/logger"
	"github.com/alphabill-org/alphabill/internal/network"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
)

const (
	defaultT3Timeout = 900 * time.Millisecond
	t3TimerID        = "t3timer"
	defaultHash      = gocrypto.SHA256
	channelBuffer    = 100
)

type (
	PartitionStore interface {
		GetSystemDescription(id p.SystemIdentifier) (*genesis.SystemDescriptionRecord, error)
	}

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	consensusConfig struct {
		hashAlgo   gocrypto.Hash
		t3Timeout  time.Duration
		stateStore StateStore
	}

	ConsensusManager struct {
		ctx          context.Context
		ctxCancel    context.CancelFunc
		certReqCh    chan consensus.IRChangeRequest
		certResultCh chan certificates.UnicityCertificate
		timers       *timer.Timers
		conf         *consensusConfig
		selfId       string // node identifier
		partitions   PartitionStore
		ir           map[p.SystemIdentifier]*certificates.InputRecord
		changes      map[p.SystemIdentifier]*certificates.InputRecord
		signer       crypto.Signer // private key of the root chain
		trustBase    map[string]crypto.Verifier
	}

	Option          func(c *consensusConfig)
	UnicitySealFunc func(rootHash []byte) (*certificates.UnicitySeal, error)
)

func WithT3Timeout(timeout time.Duration) Option {
	return func(c *consensusConfig) {
		c.t3Timeout = timeout
	}
}

func WithStateStorage(store StateStore) Option {
	return func(c *consensusConfig) {
		c.stateStore = store
	}
}

func WithHashAlgo(algo gocrypto.Hash) Option {
	return func(c *consensusConfig) {
		c.hashAlgo = algo
	}
}

func trackExecutionTime(start time.Time, name string) {
	logger.Debug(name, " took ", time.Since(start))
}

func loadInputRecords(state *store.RootState) map[p.SystemIdentifier]*certificates.InputRecord {
	ir := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for id, uc := range state.Certificates {
		ir[id] = uc.InputRecord
	}
	return ir
}

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(peer *network.Peer, partitionStore PartitionStore,
	signer crypto.Signer, opts ...Option) (*ConsensusManager, error) {
	if peer == nil {
		return nil, errors.New("peer is nil")
	}
	selfId := peer.ID().String()
	log.SetContext(log.KeyNodeID, selfId)
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	config := loadConf(opts)
	timers := timer.NewTimers()
	// verify that we can read the persisted state from store
	state, err := config.stateStore.Get()
	if err != nil {
		return nil, err
	}
	consensusManager := &ConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest, channelBuffer),
		certResultCh: make(chan certificates.UnicityCertificate, channelBuffer),
		timers:       timers,
		conf:         config,
		selfId:       selfId,
		partitions:   partitionStore,
		ir:           loadInputRecords(&state),
		changes:      make(map[p.SystemIdentifier]*certificates.InputRecord),
		signer:       signer,
		trustBase:    map[string]crypto.Verifier{selfId: verifier},
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) Start() {
	// Start T3 timer
	x.timers.Start(t3TimerID, x.conf.t3Timeout)
	go x.loop()
}

func (x *ConsensusManager) Stop() {
	x.timers.WaitClose()
	x.ctxCancel()
}

func (x *ConsensusManager) loop() {
	for {
		select {
		case <-x.ctx.Done():
			logger.Info("Exiting consensus manager main loop")
			return
		case req, ok := <-x.certReqCh:
			if !ok {
				logger.Warning("certification channel closed, exiting consensus main loop")
				return
			}
			ir, found := x.changes[req.SystemIdentifier]
			if found {
				logger.Debug("Partition %X, pending request exists %v, ignoring new %v", req.SystemIdentifier,
					ir, req.IR)
				break
			}
			logger.Debug("Partition %X, IR change request received")
			x.changes[req.SystemIdentifier] = req.IR
		// handle timeouts
		case nt := <-x.timers.C:
			if nt == nil {
				continue
			}
			timerId := nt.Name()
			switch {
			case timerId == t3TimerID:
				logger.Debug("T3 timeout")
				x.timers.Restart(timerId)
				x.onT3Timeout()
			}
		}
	}
}

func (x *ConsensusManager) onT3Timeout() {
	defer trackExecutionTime(time.Now(), fmt.Sprintf("t3 timeout handling"))
	state, err := x.conf.stateStore.Get()
	if err != nil {
		logger.Warning("T3 timeout, failed to read last state from storage: %v", err.Error())
		return
	}
	newState, err := x.generateUnicityCertificates(&state)
	if err != nil {
		logger.Warning("T3 timeout round %v failed: %v", state.LatestRound+1, err)
		// restore input records form store
		x.ir = loadInputRecords(&state)
		return
	}
	// persist new state
	if err := x.conf.stateStore.Save(*newState); err != nil {
		logger.Warning("T3 timeout: failed to persist new root state, %v", err)
		return
	}
	// Only deliver updated (new input or repeat) certificates
	for id, cert := range newState.Certificates {
		logger.Debug("T3 timeout: sending new UC for '%X'", id.Bytes())
		x.certResultCh <- *cert
	}
}

func (x *ConsensusManager) checkT2Timeout(round uint64, state *store.RootState) error {
	// evaluate timeouts
	for id, cert := range state.Certificates {
		// if new input was this partition id was not received for this round
		if _, found := x.changes[id]; !found {
			partInfo, err := x.partitions.GetSystemDescription(id)
			if err != nil {
				return err
			}
			if time.Duration(round-cert.UnicitySeal.RootChainRoundNumber)*x.conf.t3Timeout >
				time.Duration(partInfo.T2Timeout)*time.Millisecond {
				// timeout
				logger.Debug("Round %v, partition %X T2 timeout", round, id.Bytes())
				x.changes[id] = cert.InputRecord
			}
		}
	}
	return nil
}

func (x *ConsensusManager) generateUnicityCertificates(lastState *store.RootState) (*store.RootState, error) {
	newRound := lastState.LatestRound + 1
	// evaluate timeouts and add repeat UC requests if timeout
	if err := x.checkT2Timeout(newRound, lastState); err != nil {
		return nil, err
	}
	// log all changes for this round
	logger.Debug("Round %v, changed input records are:")
	// apply changes
	for id, ch := range x.changes {
		// add some sanity checks
		// log and store
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Partition %X IR:", id), ch)
		x.ir[id] = ch
	}
	utData := make([]*unicitytree.Data, 0, len(x.ir))
	// remember system description records hashes and system id for verification
	sdrhs := make(map[p.SystemIdentifier][]byte, len(x.ir))
	for id, ir := range x.ir {
		partInfo, err := x.partitions.GetSystemDescription(id)
		if err != nil {
			return nil, err
		}
		sdrh := partInfo.Hash(x.conf.hashAlgo)
		sdrhs[id] = sdrh
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            partInfo.SystemIdentifier,
			InputRecord:                 ir,
			SystemDescriptionRecordHash: sdrh,
		})
	}
	certs := make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	// create unicity tree
	ut, err := unicitytree.New(x.conf.hashAlgo.New(), utData)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	uSeal := &certificates.UnicitySeal{
		RootChainRoundNumber: newRound,
		PreviousHash:         lastState.LatestRootHash,
		Hash:                 rootHash,
		RoundCreationTime:    util.MakeTimestamp(),
	}
	if err := uSeal.Sign(x.selfId, x.signer); err != nil {
		return nil, err
	}
	// extract certificates for all changed IR's
	for sysId, ir := range x.changes {
		// get certificate for change
		utCert, err := ut.GetCertificate(sysId.Bytes())
		if err != nil {
			return nil, err
		}
		uc := &certificates.UnicityCertificate{
			InputRecord: ir,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
			UnicitySeal: uSeal,
		}
		// verify certificate
		// ignore error, we just put it there and if not, then verify will fail anyway
		srdh, _ := sdrhs[sysId]
		if err := uc.IsValid(x.trustBase, x.conf.hashAlgo, sysId.Bytes(), srdh); err != nil {
			// should never happen.
			return nil, fmt.Errorf("error invalid genese unicity certificate: %w", err)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", sysId.Bytes()), uc)
		certs[sysId] = uc
	}
	// Persist all changes
	newState := store.RootState{LatestRound: newRound, Certificates: certs, LatestRootHash: rootHash}
	// clear changed and return new certificates
	x.changes = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return &newState, nil
}

func loadConf(opts []Option) *consensusConfig {
	conf := &consensusConfig{
		hashAlgo:   defaultHash,
		t3Timeout:  defaultT3Timeout,
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
