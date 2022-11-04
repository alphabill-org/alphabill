package monolithic_consensus

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
	// todo: consider response handling taking time
	ChannelBuffer = 10
)

type (
	PartitionStore interface {
		NodeCount(id p.SystemIdentifier) int
		GetNodes(id p.SystemIdentifier) ([]string, error)
		GetTrustBase(id p.SystemIdentifier) (map[string]crypto.Verifier, error)
		GetSystemDescription(id p.SystemIdentifier) (*genesis.SystemDescriptionRecord, error)
		GetSystemDescriptions() []*genesis.SystemDescriptionRecord
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

	MonolithicConsensusManager struct {
		ctx          context.Context
		ctxCancel    context.CancelFunc
		certReqCh    chan consensus.IRChangeRequest
		certResultCh chan certificates.UnicityCertificate
		timers       *timer.Timers
		conf         *consensusConfig
		selfId       string // node identifier
		partitions   PartitionStore
		inputRecords map[p.SystemIdentifier]*certificates.InputRecord
		signer       crypto.Signer // private key of the root chain
		verifier     map[string]crypto.Verifier
	}

	Option func(c *consensusConfig)
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

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(peer *network.Peer, partitionStore PartitionStore,
	signer crypto.Signer, opts ...Option) (*MonolithicConsensusManager, error) {
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

	consensusManager := &MonolithicConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest, ChannelBuffer),
		certResultCh: make(chan certificates.UnicityCertificate, ChannelBuffer),
		timers:       timers,
		conf:         config,
		selfId:       selfId,
		partitions:   partitionStore,
		inputRecords: make(map[p.SystemIdentifier]*certificates.InputRecord),
		signer:       signer,
		verifier:     map[string]crypto.Verifier{selfId: verifier},
	}
	consensusManager.ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	return consensusManager, nil
}

func (x *MonolithicConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *MonolithicConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *MonolithicConsensusManager) Start() {
	// Start timers
	x.timers.Start(t3TimerID, x.conf.t3Timeout)
	// todo: should refactor to use round number or UC seal timestamp for partition timeouts
	for _, sysDesc := range x.partitions.GetSystemDescriptions() {
		duration := time.Duration(sysDesc.T2Timeout) * time.Millisecond
		x.timers.Start(string(sysDesc.SystemIdentifier), duration)
	}
	go x.loop()
}

func (x *MonolithicConsensusManager) Stop() {
	x.timers.WaitClose()
	x.ctxCancel()
}

func (x *MonolithicConsensusManager) loop() {
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
			logger.Debug("IR change request from partition")
			// The request is sent internally, assume correct handling and do not double-check the attached requests
			x.inputRecords[req.SystemIdentifier] = &req.IR
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
				newState, err := x.CreateUnicityCertificates()
				if err != nil {
					logger.Warning("Round %v failed: %v", newState.LatestRound, err)
					break
				}
				for id, cert := range newState.Certificates {
					logger.Debug("Sending new UC for '%X'", []byte(id))
					x.certResultCh <- *cert
				}
			default:
				logger.Debug("Handling T2 timeout with a name '%X'", []byte(timerId))
				state, err := x.conf.stateStore.Get()
				if err != nil {
					logger.Warning("Unable to re-certify partition %X, error: %v", []byte(timerId), err.Error())
					break
				}
				luc, f := state.Certificates[p.SystemIdentifier(timerId)]
				if !f {
					logger.Warning("Unable to re-certify partition %X, error: no certificate found", []byte(timerId), err.Error())
					break
				}
				x.inputRecords[p.SystemIdentifier(timerId)] = luc.InputRecord
				x.timers.Restart(timerId)
			}
		}
	}
}

func (x *MonolithicConsensusManager) CreateUnicityCertificates() (*store.RootState, error) {
	data := x.toUnicityTreeData(x.inputRecords)
	logger.Debug("Input records are:")
	for _, ir := range data {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("IR for partition %X is:", ir.SystemIdentifier), ir)
	}
	ut, err := unicitytree.New(x.conf.hashAlgo.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	logger.Info("New root hash is %X", rootHash)
	lastState, err := x.conf.stateStore.Get()
	if err != nil {
		logger.Info("Failed to read last state from storage: %v", err.Error())
		return nil, err
	}
	newRound := lastState.LatestRound + 1
	unicitySeal, err := x.createUnicitySeal(newRound, rootHash, lastState.LatestRootHash)
	if err != nil {
		return nil, err
	}
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", newRound, len(data))

	var certs = make(map[p.SystemIdentifier]*certificates.UnicityCertificate)
	for _, d := range data {
		cert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		identifier := p.SystemIdentifier(d.SystemIdentifier)
		sysDesc, err := x.partitions.GetSystemDescription(identifier)
		if err != nil {
			logger.Warning("Unexpected unknown partition id %X", sysDesc)
		}
		sdrHash := sysDesc.Hash(x.conf.hashAlgo)

		certificate := &certificates.UnicityCertificate{
			InputRecord: d.InputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      cert.SystemIdentifier,
				SiblingHashes:         cert.SiblingHashes,
				SystemDescriptionHash: sdrHash,
			},
			UnicitySeal: unicitySeal,
		}

		// check the certificate
		err = certificate.IsValid(x.verifier, x.conf.hashAlgo, d.SystemIdentifier, d.SystemDescriptionRecordHash)
		if err != nil {
			// should never happen.
			panic(err)
		}
		certs[identifier] = certificate
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)
	}
	// Save state
	newState := store.RootState{LatestRound: newRound, Certificates: certs, LatestRootHash: rootHash}
	if err := x.conf.stateStore.Save(newState); err != nil {
		return nil, err
	}
	// clear input records, all that could were handled and certificates created
	x.inputRecords = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return &newState, nil
}

func (x *MonolithicConsensusManager) toUnicityTreeData(records map[p.SystemIdentifier]*certificates.InputRecord) []*unicitytree.Data {
	data := make([]*unicitytree.Data, len(records))
	i := 0
	for key, r := range records {
		systemDescriptionRecord, _ := x.partitions.GetSystemDescription(key)
		data[i] = &unicitytree.Data{
			SystemIdentifier:            systemDescriptionRecord.SystemIdentifier,
			InputRecord:                 r,
			SystemDescriptionRecordHash: systemDescriptionRecord.Hash(x.conf.hashAlgo),
		}
		i++
	}
	return data
}

func (x *MonolithicConsensusManager) createUnicitySeal(newRound uint64, newRootHash []byte, prevRoot []byte) (*certificates.UnicitySeal, error) {
	u := &certificates.UnicitySeal{
		RootChainRoundNumber: newRound,
		PreviousHash:         prevRoot,
		Hash:                 newRootHash,
	}
	return u, u.Sign(x.selfId, x.signer)
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
