package monolithic

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	log "github.com/alphabill-org/alphabill/internal/logger"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"golang.org/x/exp/maps"
)

const (
	defaultT3Timeout = 900 * time.Millisecond
	t3TimerID        = "t3timer"
	defaultHash      = gocrypto.SHA256
)

type (
	PartitionStore interface {
		GetPartitionInfo(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	StateStore interface {
		Save(state store.RootState) error
		Get() (store.RootState, error)
	}

	consensusConfig struct {
		hashAlgo  gocrypto.Hash
		t3Timeout time.Duration
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
		stateStore   StateStore
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

func WithHashAlgo(algo gocrypto.Hash) Option {
	return func(c *consensusConfig) {
		c.hashAlgo = algo
	}
}

func trackExecutionTime(start time.Time, name string) {
	logger.Debug(fmt.Sprintf("%v took %v", name, time.Since(start)))
}

func loadInputRecords(state *store.RootState) map[p.SystemIdentifier]*certificates.InputRecord {
	ir := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for id, uc := range state.Certificates {
		ir[id] = uc.InputRecord
	}
	return ir
}

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(selfId string, partitionStore PartitionStore, stateStore StateStore,
	signer crypto.Signer, opts ...Option) (*ConsensusManager, error) {
	log.SetContext(log.KeyNodeID, selfId)
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, err
	}
	config := loadConf(opts)
	timers := timer.NewTimers()
	// verify that we can read the persisted state from store
	state, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	consensusManager := &ConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest),
		certResultCh: make(chan certificates.UnicityCertificate),
		timers:       timers,
		conf:         config,
		selfId:       selfId,
		partitions:   partitionStore,
		stateStore:   stateStore,
		ir:           loadInputRecords(&state),
		changes:      make(map[p.SystemIdentifier]*certificates.InputRecord),
		signer:       signer,
		trustBase:    map[string]crypto.Verifier{selfId: verifier},
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	consensusManager.start()
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) start() {
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
				logger.Warning("Certification channel closed, exiting consensus main loop")
				return
			}
			ir, found := x.changes[req.SystemIdentifier]
			if found {
				logger.Debug("Partition %X, pending request exists %v, ignoring new %v", req.SystemIdentifier,
					ir, req.IR)
				break
			}
			logger.Debug("Partition %X, IR change request received", req.SystemIdentifier)
			x.changes[req.SystemIdentifier] = req.IR
		// handle timeouts
		case nt := <-x.timers.C:
			if nt == nil {
				continue
			}
			timerId := nt.Name()
			switch {
			case timerId == t3TimerID:
				logger.Info("T3 timeout")
				x.timers.Restart(timerId)
				x.onT3Timeout()
			}
		}
	}
}

func (x *ConsensusManager) onT3Timeout() {
	defer trackExecutionTime(time.Now(), "t3 timeout handling")
	state, err := x.stateStore.Get()
	if err != nil {
		logger.Warning("T3 timeout, failed to read last state from storage: %v", err.Error())
		return
	}
	newRound := state.LatestRound + 1
	// evaluate timeouts and add repeat UC requests if timeout
	if err := x.checkT2Timeout(newRound, &state); err != nil {
		return
	}
	// if no new consensus or timeout then skip the round
	if len(x.changes) == 0 {
		logger.Info("Round %v, no IR changes", newRound)
		// persist new round
		newState := store.RootState{
			LatestRound:    newRound,
			LatestRootHash: state.LatestRootHash,
		}
		if err := x.stateStore.Save(newState); err != nil {
			logger.Warning("Round %d failed to persist new root state, %v", newState.LatestRound, err)
			return
		}
		return
	}
	newState, err := x.generateUnicityCertificates(newRound, &state)
	if err != nil {
		logger.Warning("Round %d, T3 timeout failed: %v", state.LatestRound+1, err)
		// restore input records form last state
		x.ir = loadInputRecords(&state)
		return
	}
	// persist new state
	if err := x.stateStore.Save(*newState); err != nil {
		logger.Warning("Round %d failed to persist new root state, %v", newState.LatestRound, err)
		return
	}
	// Only deliver updated (new input or repeat) certificates
	for id, cert := range newState.Certificates {
		logger.Debug("Round %d sending new UC for '%X'", newState.LatestRound, id.Bytes())
		x.certResultCh <- *cert
	}
}

func (x *ConsensusManager) checkT2Timeout(round uint64, state *store.RootState) error {
	// evaluate timeouts
	for id, cert := range state.Certificates {
		// if new input was this partition id was not received for this round
		if _, found := x.changes[id]; !found {
			partInfo, err := x.partitions.GetPartitionInfo(id)
			if err != nil {
				return err
			}
			if time.Duration(round-cert.UnicitySeal.RootRoundInfo.RoundNumber)*x.conf.t3Timeout >
				time.Duration(partInfo.SystemDescription.T2Timeout)*time.Millisecond {
				// timeout
				logger.Info("Round %v, partition %X T2 timeout", round, id.Bytes())
				x.changes[id] = cert.InputRecord
			}
		}
	}
	return nil
}

func (x *ConsensusManager) generateUnicityCertificates(round uint64, lastState *store.RootState) (*store.RootState, error) {
	// log all changes for this round
	logger.Info("Round %v, certify changes for partitions %X", round, maps.Keys(x.changes))
	// apply changes
	for id, ch := range x.changes {
		// add sanity checks
		// log and store
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Partition %X IR:", id), ch)
		x.ir[id] = ch
	}
	utData := make([]*unicitytree.Data, 0, len(x.ir))
	for id, ir := range x.ir {
		partInfo, err := x.partitions.GetPartitionInfo(id)
		if err != nil {
			return nil, err
		}
		sdrh := partInfo.SystemDescription.Hash(x.conf.hashAlgo)
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            partInfo.SystemDescription.SystemIdentifier,
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
	roundMeta := &certificates.RootRoundInfo{
		RoundNumber:       round,
		Epoch:             0,
		Timestamp:         util.MakeTimestamp(),
		ParentRoundNumber: round - 1,
		CurrentRootHash:   rootHash,
	}
	uSeal := &certificates.UnicitySeal{
		RootRoundInfo: roundMeta,
		CommitInfo: &certificates.CommitInfo{
			RootRoundInfoHash: roundMeta.Hash(gocrypto.SHA256),
			RootHash:          rootHash,
		},
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
		if err := uc.IsValid(x.trustBase, x.conf.hashAlgo, utCert.SystemIdentifier, utCert.SystemDescriptionHash); err != nil {
			// should never happen.
			return nil, fmt.Errorf("error invalid generated unicity certificate: %w", err)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", sysId.Bytes()), uc)
		certs[sysId] = uc
	}
	// Persist all changes
	newState := store.RootState{LatestRound: round, Certificates: certs, LatestRootHash: rootHash}
	// clear changed and return new certificates
	x.changes = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return &newState, nil
}

func loadConf(opts []Option) *consensusConfig {
	conf := &consensusConfig{
		hashAlgo:  defaultHash,
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
