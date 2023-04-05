package monolithic

import (
	"context"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	p "github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
	"golang.org/x/exp/maps"
)

type (
	ConsensusManager struct {
		ctxCancel    context.CancelFunc
		certReqCh    chan consensus.IRChangeRequest
		certResultCh chan certificates.UnicityCertificate
		params       *consensus.Parameters
		ticker       *time.Ticker
		selfID       string // node identifier
		partitions   partitions.PartitionConfiguration
		stateStore   *StateStore
		round        uint64
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

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(selfStr string, rg *genesis.RootGenesis, partitionStore partitions.PartitionConfiguration,
	signer crypto.Signer, opts ...consensus.Option) (*ConsensusManager, error) {
	verifier, err := signer.Verifier()
	if err != nil {
		return nil, fmt.Errorf("signing key error, %w", err)
	}
	// load optional parameters
	optional := consensus.LoadConf(opts)
	// Initiate store
	storage, err := NewStateStore(optional.StoragePath)
	if err != nil {
		return nil, fmt.Errorf("storage init failed, %w", err)
	}
	if storage.IsEmpty() {
		// init form genesis
		logger.Info("Consensus init from genesis")
		if err = storage.Init(rg); err != nil {
			return nil, fmt.Errorf("consneus manager genesis init failed, %w", err)
		}
	}
	lastIR, err := storage.GetLastCertifiedInputRecords()
	if err != nil {
		return nil, fmt.Errorf("restore root state from DB failed, %w", err)
	}
	lastRound, err := storage.GetRound()
	if err != nil {
		return nil, fmt.Errorf("restore root round from DB failed, %w", err)
	}
	consensusParams := consensus.NewConsensusParams(rg.Root)
	consensusManager := &ConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest),
		certResultCh: make(chan certificates.UnicityCertificate),
		params:       consensusParams,
		ticker:       time.NewTicker(consensusParams.BlockRateMs),
		selfID:       selfStr,
		partitions:   partitionStore,
		stateStore:   storage,
		round:        lastRound,
		ir:           lastIR,
		changes:      make(map[p.SystemIdentifier]*certificates.InputRecord),
		signer:       signer,
		trustBase:    map[string]crypto.Verifier{selfStr: verifier},
	}
	var ctx context.Context
	ctx, consensusManager.ctxCancel = context.WithCancel(context.Background())
	go consensusManager.loop(ctx)
	return consensusManager, nil
}

func (x *ConsensusManager) RequestCertification() chan<- consensus.IRChangeRequest {
	return x.certReqCh
}

func (x *ConsensusManager) CertificationResult() <-chan certificates.UnicityCertificate {
	return x.certResultCh
}

func (x *ConsensusManager) Stop() {
	x.ticker.Stop()
	x.ctxCancel()
}

func (x *ConsensusManager) GetLatestUnicityCertificate(id p.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	luc, err := x.stateStore.GetCertificate(id)
	if err != nil {
		return nil, fmt.Errorf("find certificate for system id %X failed, %w", id, err)
	}
	return luc, nil
}

func (x *ConsensusManager) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
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
		case _, ok := <-x.ticker.C:
			if !ok {
				logger.Warning("Ticker channel closed, exiting main loop")
				return
			}
			x.onT3Timeout()
		}
	}
}

// onIRChangeReq handles partition IR change requests.
// NB! the request is received from validator go routine running in the same instance.
// Hence, it is assumed that the partition request handler is working correctly and the proof is not double verified here
func (x *ConsensusManager) onIRChangeReq(req *consensus.IRChangeRequest) error {
	var newInputRecord *certificates.InputRecord = nil
	switch req.Reason {
	case consensus.Quorum:
		// simple sanity check
		if len(req.Requests) == 0 {
			return fmt.Errorf("error invalid quorum proof, no requests")
		}
		newInputRecord = req.Requests[0].InputRecord
	case consensus.QuorumNotPossible:
		luc, err := x.stateStore.GetCertificate(req.SystemIdentifier)
		if err != nil {
			return fmt.Errorf("ir change request ignored, read state for system id %X failed, %w", req.SystemIdentifier.Bytes(), err)
		}
		// repeat UC, ignore error here as we found the luc, and it cannot be nil
		newInputRecord, _ = certificates.NewRepeatInputRecord(luc.InputRecord)
	default:
		return fmt.Errorf("invalid certfification reason %v", req.Reason)
	}
	// In this round, has a request already been received?
	_, found := x.changes[req.SystemIdentifier]
	// ignore duplicate request, first come, first served
	// should probably be more vocal if this not a binary duplicate
	if found {
		logger.Debug("Partition %X, pending request exists, ignoring new", req.SystemIdentifier)
		return nil
	}
	logger.Debug("Partition %X, IR change request received", req.SystemIdentifier)
	x.changes[req.SystemIdentifier] = newInputRecord
	return nil
}

func (x *ConsensusManager) onT3Timeout() {
	defer trackExecutionTime(time.Now(), "t3 timeout handling")
	logger.Info("T3 timeout")
	// increment
	newRound := x.round + 1
	// evaluate timeouts and add repeat UC requests if timeout
	if err := x.checkT2Timeout(newRound); err != nil {
		return
	}
	certs, err := x.generateUnicityCertificates(newRound)
	if err != nil {
		logger.Warning("Round %d, T3 timeout failed: %v", newRound, err)
		return
	}
	// update local cache for round number
	x.round = newRound
	// Only deliver updated (new input or repeat) certificates
	for id, cert := range certs {
		logger.Debug("Round %d sending new UC for '%X'", newRound, id.Bytes())
		x.certResultCh <- *cert
	}
}

func (x *ConsensusManager) checkT2Timeout(round uint64) error {
	// evaluate timeouts
	for id := range x.ir {
		// if new input was this partition id was not received for this round
		if _, found := x.changes[id]; !found {
			partInfo, _, err := x.partitions.GetInfo(id)
			if err != nil {
				return err
			}
			lastCert, err := x.stateStore.GetCertificate(id)
			if err != nil {
				logger.Warning("Unexpected error, read certificate for %X failed, %v", id.Bytes(), err)
				continue
			}
			if time.Duration(round-lastCert.UnicitySeal.RootChainRoundNumber)*x.params.BlockRateMs >
				time.Duration(partInfo.T2Timeout)*time.Millisecond {
				// timeout
				logger.Info("Round %v, partition %X T2 timeout", round, id.Bytes())
				repeatIR, _ := certificates.NewRepeatInputRecord(lastCert.InputRecord)
				x.changes[id] = repeatIR
			}
		}
	}
	return nil
}

func getMergeInputRecords(currentIR, changed map[p.SystemIdentifier]*certificates.InputRecord) map[p.SystemIdentifier]*certificates.InputRecord {
	result := make(map[p.SystemIdentifier]*certificates.InputRecord)
	for id, ir := range currentIR {
		result[id] = ir
	}
	for id, ch := range changed {
		result[id] = ch
		// trace level log for more details
		util.WriteTraceJsonLog(logger, fmt.Sprintf("Partition %X IR:", id), ch)
	}
	return result
}

// generateUnicityCertificates generates certificates for all changed input records in round
func (x *ConsensusManager) generateUnicityCertificates(round uint64) (map[p.SystemIdentifier]*certificates.UnicityCertificate, error) {
	// if no new consensus or timeouts then skip the round
	if len(x.changes) == 0 {
		logger.Info("Round %v, no IR changes", round)
		// persist new round
		if err := x.stateStore.Update(round, nil); err != nil {
			return nil, fmt.Errorf("round %v failed to persist new root round, %w", round, err)
		}
		return nil, nil
	}
	// log all changes for this round
	logger.Info("Round %v, certify changes for partitions %X", round, maps.Keys(x.changes))
	// merge changed and unchanged input records and create unicity tree from the whole set
	newIR := getMergeInputRecords(x.ir, x.changes)
	// convert IR to unicity tree input
	utData := make([]*unicitytree.Data, 0, len(newIR))
	for id, rec := range newIR {
		sysDesc, _, err := x.partitions.GetInfo(id)
		if err != nil {
			return nil, err
		}
		sdrh := sysDesc.Hash(x.params.HashAlgorithm)
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            sysDesc.SystemIdentifier,
			InputRecord:                 rec,
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
	uSeal := &certificates.UnicitySeal{
		RootChainRoundNumber: round,
		Hash:                 rootHash,
	}
	if err = uSeal.Sign(x.selfID, x.signer); err != nil {
		return nil, err
	}
	// extract certificates for all changed IR's
	for sysID, ir := range x.changes {
		// get certificate for change
		var utCert *certificates.UnicityTreeCertificate
		utCert, err = ut.GetCertificate(sysID.Bytes())
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
		util.WriteTraceJsonLog(logger, fmt.Sprintf("NewStateStore unicity certificate for partition %X is", sysID.Bytes()), uc)
		certs[sysID] = uc
	}
	// persist new state
	if err = x.stateStore.Update(round, certs); err != nil {
		return nil, fmt.Errorf("round %v failed to persist new root state, %w", round, err)
	}
	// now that everything is successfully stored, persist changes to input records
	for id, ir := range x.changes {
		x.ir[id] = ir
	}
	// clear changed and return new certificates
	x.changes = make(map[p.SystemIdentifier]*certificates.InputRecord)
	return certs, nil
}
