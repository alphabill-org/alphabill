package monolithic

import (
	"context"
	gocrypto "crypto"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/timer"
	"github.com/alphabill-org/alphabill/internal/util"
	"golang.org/x/exp/maps"
)

const (
	t3TimerID = "t3timer"
)

type (
	PartitionStore interface {
		Info(id p.SystemIdentifier) (partition_store.PartitionInfo, error)
	}

	Store interface {
		Save(*RootState) error
		Get() (*RootState, error)
	}

	ConsensusManager struct {
		ctx          context.Context
		ctxCancel    context.CancelFunc
		certReqCh    chan consensus.IRChangeRequest
		certResultCh chan certificates.UnicityCertificate
		params       *consensus.Parameters
		timers       *timer.Timers
		selfID       string // node identifier
		partitions   PartitionStore
		stateStore   *StateStore
		ir           map[p.SystemIdentifier]*certificates.InputRecord
		changes      map[p.SystemIdentifier]*certificates.InputRecord
		signer       crypto.Signer // private key of the root chain
		trustBase    map[string]crypto.Verifier
	}

	UnicitySealFunc func(rootHash []byte) (*certificates.UnicitySeal, error)
)

func trackExecutionTime(start time.Time, name string) {
	logger.Debug(fmt.Sprintf("%v took %v", name, time.Since(start)))
}

func loadInputRecords(state *RootState) map[p.SystemIdentifier]*certificates.InputRecord {
	ir := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for id, uc := range state.Certificates {
		ir[id] = uc.InputRecord
	}
	return ir
}

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(selfStr string, rg *genesis.RootGenesis, partitionStore PartitionStore,
	signer crypto.Signer, opts ...consensus.Option) (*ConsensusManager, error) {
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, fmt.Errorf("signing key error, %w", err)
	}
	// load optional parameters
	optional := consensus.LoadConf(opts)
	// Initiate store
	storage, err := NewStateStore(rg, optional.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("consneus manager storage init failed, %w", err)
	}
	lastState := storage.Get()
	consensusManager := &ConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest),
		certResultCh: make(chan certificates.UnicityCertificate),
		params:       consensus.NewConsensusParams(rg.Root),
		timers:       timer.NewTimers(),
		selfID:       selfStr,
		partitions:   partitionStore,
		stateStore:   storage,
		ir:           loadInputRecords(lastState),
		changes:      make(map[p.SystemIdentifier]*certificates.InputRecord),
		signer:       signer,
		trustBase:    map[string]crypto.Verifier{selfStr: verifier},
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
	x.timers.Start(t3TimerID, x.params.BlockRateMs)
	go x.loop()
}

func (x *ConsensusManager) Stop() {
	x.timers.WaitClose()
	x.ctxCancel()
}

func (x *ConsensusManager) GetLatestUnicityCertificate(id p.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	state := x.stateStore.Get()
	luc, f := state.Certificates[id]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %X", id)
	}
	return luc, nil
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
			if err := x.onIRChangeReq(&req); err != nil {
				logger.Warning("Certification request error, %w", err)
			}
		// handle timeouts
		case nt, ok := <-x.timers.C:
			if !ok {
				logger.Warning("Timers channel closed, exiting main loop")
				return
			}
			if nt == nil {
				continue
			}
			timerID := nt.Name()
			switch {
			case timerID == t3TimerID:
				logger.Info("T3 timeout")
				x.timers.Restart(timerID)
				x.onT3Timeout()
			}
		}
	}
}

// onIRChangeReq handles partition IR change requests.
// NB! this is a test implementation, which receives the request from validator go routine running in the same instance.
// Hence, we assume that the partition request handler is working correctly and do not check the proof here.
// More correct would be to
func (x *ConsensusManager) onIRChangeReq(req *consensus.IRChangeRequest) error {
	var newInputRecord *certificates.InputRecord = nil
	switch req.Reason {
	case consensus.Quorum:
		// simple sanity check
		if len(req.Requests) == 0 {
			return fmt.Errorf("error invalid quorum proof, no requests")
		}
		newInputRecord = req.Requests[0].InputRecord
		break
	case consensus.QuorumNotPossible:
		state := x.stateStore.Get()
		luc, f := state.Certificates[req.SystemIdentifier]
		if !f {
			return fmt.Errorf("ir change request ignored, no last state for system id %X", req.SystemIdentifier.Bytes())
		}
		// repeat UC
		// todo: AB-505 add partition round number increment and 'nullhash' for block, epoch check
		newInputRecord = luc.InputRecord
		break

	default:
		return fmt.Errorf("invalid certfification reason %v", req.Reason)
	}

	ir, found := x.changes[req.SystemIdentifier]

	if found {
		logger.Debug("Partition %X, pending request exists %v, ignoring new %v", req.SystemIdentifier,
			ir, newInputRecord)
		return nil
	}
	logger.Debug("Partition %X, IR change request received", req.SystemIdentifier)
	x.changes[req.SystemIdentifier] = newInputRecord
	return nil
}

func (x *ConsensusManager) onT3Timeout() {
	defer trackExecutionTime(time.Now(), "t3 timeout handling")
	lastState := x.stateStore.Get()
	newRound := lastState.Round + 1
	// evaluate timeouts and add repeat UC requests if timeout
	if err := x.checkT2Timeout(newRound, lastState); err != nil {
		return
	}
	// if no new consensus or timeout then skip the round
	if len(x.changes) == 0 {
		logger.Info("Round %v, no IR changes", newRound)
		// persist new round
		newState := &RootState{
			Round:    newRound,
			RootHash: lastState.RootHash,
		}
		if err := x.stateStore.Save(newState); err != nil {
			logger.Warning("Round %d failed to persist new root state, %v", newState.Round, err)
			return
		}
		return
	}
	newState, err := x.generateUnicityCertificates(newRound)
	if err != nil {
		logger.Warning("Round %d, T3 timeout failed: %v", newRound, err)
		// restore input records form last state
		x.ir = loadInputRecords(lastState)
		return
	}
	// persist new state
	if err = x.stateStore.Save(newState); err != nil {
		logger.Warning("Round %d failed to persist new root state, %v", newState.Round, err)
		return
	}
	// Only deliver updated (new input or repeat) certificates
	for id, cert := range newState.Certificates {
		logger.Debug("Round %d sending new UC for '%X'", newState.Round, id.Bytes())
		x.certResultCh <- *cert
	}
}

func (x *ConsensusManager) checkT2Timeout(round uint64, state *RootState) error {
	// evaluate timeouts
	for id, cert := range state.Certificates {
		// if new input was this partition id was not received for this round
		if _, found := x.changes[id]; !found {
			partInfo, err := x.partitions.Info(id)
			if err != nil {
				return err
			}
			if time.Duration(round-cert.UnicitySeal.RootRoundInfo.RoundNumber)*x.params.BlockRateMs >
				time.Duration(partInfo.SystemDescription.T2Timeout)*time.Millisecond {
				// timeout
				logger.Info("Round %v, partition %X T2 timeout", round, id.Bytes())
				x.changes[id] = cert.InputRecord
			}
		}
	}
	return nil
}

func (x *ConsensusManager) generateUnicityCertificates(round uint64) (*RootState, error) {
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
		partInfo, err := x.partitions.Info(id)
		if err != nil {
			return nil, err
		}
		sdrh := partInfo.SystemDescription.Hash(x.params.HashAlgorithm)
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
	ut, err := unicitytree.New(x.params.HashAlgorithm.New(), utData)
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
	if err = uSeal.Sign(x.selfID, x.signer); err != nil {
		return nil, err
	}
	// extract certificates for all changed IR's
	for sysID, ir := range x.changes {
		// get certificate for change
		utCert, err := ut.GetCertificate(sysID.Bytes())
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
		if err = uc.IsValid(x.trustBase, x.params.HashAlgorithm, utCert.SystemIdentifier, utCert.SystemDescriptionHash); err != nil {
			// should never happen.
			return nil, fmt.Errorf("error invalid generated unicity certificate: %w", err)
		}
		util.WriteDebugJsonLog(logger, fmt.Sprintf("NewStateStore unicity certificate for partition %X is", sysID.Bytes()), uc)
		certs[sysID] = uc
	}
	// Persist all changes
	newState := RootState{Round: round, Certificates: certs, RootHash: rootHash}
	// clear changed and return new certificates
	x.changes = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return &newState, nil
}
