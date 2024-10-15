package monolithic

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
)

type (
	ConsensusManager struct {
		certReqCh    chan consensus.IRChangeRequest
		certResultCh chan *certification.CertificationResponse
		params       *consensus.Parameters
		selfID       string // node identifier
		partitions   partitions.PartitionConfiguration
		stateStore   *StateStore
		round        uint64
		ir           map[types.SystemID]*types.InputRecord
		changes      map[types.SystemID]*types.InputRecord
		signer       crypto.Signer // private key of the root chain
		trustBase    types.RootTrustBase
		log          *slog.Logger
	}

	UnicitySealFunc func(rootHash []byte) (*types.UnicitySeal, error)
)

func trackExecutionTime(start time.Time, name string, log *slog.Logger) {
	log.Debug(fmt.Sprintf("%s took %s", name, time.Since(start)))
}

// NewMonolithicConsensusManager creates new monolithic (single node) consensus manager
func NewMonolithicConsensusManager(
	selfStr string,
	trustBase types.RootTrustBase,
	rg *genesis.RootGenesis,
	partitionStore partitions.PartitionConfiguration,
	signer crypto.Signer,
	log *slog.Logger,
	opts ...consensus.Option,
) (*ConsensusManager, error) {
	// load optional parameters
	optional, err := consensus.LoadConf(opts)
	if err != nil {
		return nil, fmt.Errorf("loading optional configuration: %w", err)
	}
	// Initiate store
	storage := NewStateStore(optional.Storage)
	empty, err := storage.IsEmpty()
	if err != nil {
		return nil, fmt.Errorf("storage init db empty check failed: %w", err)
	}
	if empty {
		// init form genesis
		log.Info("Consensus init from genesis")
		if err = storage.Init(rg); err != nil {
			return nil, fmt.Errorf("consensus manager genesis init failed: %w", err)
		}
	}
	lastIR, err := storage.GetLastCertifiedInputRecords()
	if err != nil {
		return nil, fmt.Errorf("restore root state from DB failed: %w", err)
	}
	lastRound, err := storage.GetRound()
	if err != nil {
		return nil, fmt.Errorf("restore root round from DB failed: %w", err)
	}
	consensusParams := consensus.NewConsensusParams(rg.Root)
	consensusManager := &ConsensusManager{
		certReqCh:    make(chan consensus.IRChangeRequest),
		certResultCh: make(chan *certification.CertificationResponse),
		params:       consensusParams,
		selfID:       selfStr,
		partitions:   partitionStore,
		stateStore:   storage,
		round:        lastRound,
		ir:           lastIR,
		changes:      make(map[types.SystemID]*types.InputRecord),
		signer:       signer,
		trustBase:    trustBase,
		log:          log,
	}
	if err := partitionStore.Reset(func() uint64 { return consensusManager.round }); err != nil {
		return nil, fmt.Errorf("resetting partition store: %w", err)
	}
	return consensusManager, nil
}

func (x *ConsensusManager) Run(ctx context.Context) error {
	return x.loop(ctx)
}

func (x *ConsensusManager) RequestCertification(ctx context.Context, cr consensus.IRChangeRequest) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case x.certReqCh <- cr:
	}
	return nil
}

func (x *ConsensusManager) CertificationResult() <-chan *certification.CertificationResponse {
	return x.certResultCh
}

func (x *ConsensusManager) GetLatestUnicityCertificate(id types.SystemID, shard types.ShardID) (*certification.CertificationResponse, error) {
	luc, err := x.stateStore.GetCertificate(id)
	if err != nil {
		return nil, fmt.Errorf("loading certificate for partition %s from state store: %w", id, err)
	}
	return luc, nil
}

func (x *ConsensusManager) ShardInfo(partition types.SystemID, shard types.ShardID) (*drctypes.ShardInfo, error) {
	return nil, fmt.Errorf("ShardInfo not implemented")
}

func (x *ConsensusManager) loop(ctx context.Context) error {
	// start root round timer
	ticker := time.NewTicker(x.params.BlockRate)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req, ok := <-x.certReqCh:
			if !ok {
				return fmt.Errorf("certification channel closed")
			}
			if err := x.onIRChangeReq(&req); err != nil {
				x.log.WarnContext(ctx, "handling certification request", logger.Error(err))
			}
		// handle timeouts
		case <-ticker.C:
			x.onT3Timeout(ctx)
		}
	}
}

// onIRChangeReq handles partition IR change requests.
// NB! the request is received from validator go routine running in the same instance.
// Hence, it is assumed that the partition request handler is working correctly and the proof is not double verified here
func (x *ConsensusManager) onIRChangeReq(req *consensus.IRChangeRequest) error {
	var newInputRecord *types.InputRecord = nil
	switch req.Reason {
	case consensus.Quorum:
		// simple sanity check
		if len(req.Requests) == 0 {
			return fmt.Errorf("error invalid quorum proof, no requests")
		}
		newInputRecord = req.Requests[0].InputRecord
	case consensus.QuorumNotPossible:
		luc, err := x.stateStore.GetCertificate(req.Partition)
		if err != nil {
			return fmt.Errorf("ir change request ignored, read state for system id %s failed, %w", req.Partition, err)
		}
		// repeat UC, ignore error here as we found the luc, and it cannot be nil
		// in repeat UC just advance partition/shard round number
		newInputRecord = luc.UC.InputRecord.NewRepeatIR()
	default:
		return fmt.Errorf("invalid certification reason %v", req.Reason)
	}
	// In this round, has a request already been received?
	_, found := x.changes[req.Partition]
	// ignore duplicate request, first come, first served
	// should probably be more vocal if this not a binary duplicate
	if found {
		return fmt.Errorf("partition %s, pending request exists, ignoring new", req.Partition)
	}
	x.log.Debug(fmt.Sprintf("partition %s, IR change request received", req.Partition))
	x.changes[req.Partition] = newInputRecord
	return nil
}

func (x *ConsensusManager) onT3Timeout(ctx context.Context) {
	defer trackExecutionTime(time.Now(), "t3 timeout handling", x.log)
	x.log.InfoContext(ctx, "T3 timeout")
	// increment
	newRound := x.round + 1
	// evaluate timeouts and add repeat UC requests if timeout
	if err := x.checkT2Timeout(newRound); err != nil {
		return
	}
	certs, err := x.generateUnicityCertificates(newRound)
	if err != nil {
		x.log.WarnContext(ctx, "T3 timeout failed", logger.Round(newRound), logger.Error(err))
		return
	}
	// update local cache for round number
	x.round = newRound
	// Only deliver updated (new input or repeat) certificates
	for _, cert := range certs {
		x.log.DebugContext(ctx, fmt.Sprintf("sending new UC for '%s'", cert.Partition), logger.Round(newRound))
		select {
		case x.certResultCh <- cert:
		case <-ctx.Done():
			return
		}
	}
}

func (x *ConsensusManager) checkT2Timeout(round uint64) error {
	log := x.log.With(logger.Round(round))
	// evaluate timeouts
	for id := range x.ir {
		// if new input was this partition id was not received for this round
		if _, found := x.changes[id]; !found {
			partInfo, _, err := x.partitions.GetInfo(id, round)
			if err != nil {
				return err
			}
			lastCert, err := x.stateStore.GetCertificate(id)
			if err != nil {
				log.Warn(fmt.Sprintf("read certificate for %s", id), logger.Error(err))
				continue
			}
			if time.Duration(round-lastCert.UC.UnicitySeal.RootChainRoundNumber)*x.params.BlockRate > partInfo.T2Timeout {
				// timeout
				log.Info(fmt.Sprintf("partition %s T2 timeout", id))
				repeatIR := lastCert.UC.InputRecord.NewRepeatIR()
				x.changes[id] = repeatIR
			}
		}
	}
	return nil
}

func getMergeInputRecords(currentIR, changed map[types.SystemID]*types.InputRecord, log *slog.Logger) map[types.SystemID]*types.InputRecord {
	result := make(map[types.SystemID]*types.InputRecord)
	for id, ir := range currentIR {
		result[id] = ir
	}
	for id, ch := range changed {
		result[id] = ch
		// trace level log for more details
		log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("Partition %s IR", id), logger.Data(ch))
	}
	return result
}

// generateUnicityCertificates generates certificates for all changed input records in round
func (x *ConsensusManager) generateUnicityCertificates(round uint64) ([]*certification.CertificationResponse, error) {
	// if no new consensus or timeouts then skip the round
	if len(x.changes) == 0 {
		x.log.Info("no IR changes", logger.Round(round))
		// persist new round
		if err := x.stateStore.Update(round, nil); err != nil {
			return nil, fmt.Errorf("round %v failed to persist new root round, %w", round, err)
		}
		return nil, nil
	}
	// merge changed and unchanged input records and create unicity tree from the whole set
	newIR := getMergeInputRecords(x.ir, x.changes, x.log)
	// convert IR to unicity tree input
	utData := make([]*types.UnicityTreeData, 0, len(newIR))
	for id, rec := range newIR {
		sysDesc, _, err := x.partitions.GetInfo(id, round)
		if err != nil {
			return nil, err
		}
		sdrh := sysDesc.Hash(x.params.HashAlgorithm)
		utData = append(utData, &types.UnicityTreeData{
			SystemIdentifier:         sysDesc.SystemIdentifier,
			InputRecord:              rec,
			PartitionDescriptionHash: sdrh,
		})
	}
	certs := make([]*certification.CertificationResponse, 0, len(x.changes))
	// create unicity tree
	ut, err := unicitytree.New(x.params.HashAlgorithm, utData)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	uSeal := &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: round,
		Timestamp:            types.NewTimestamp(),
		Hash:                 rootHash,
	}
	if err = uSeal.Sign(x.selfID, x.signer); err != nil {
		return nil, err
	}
	// extract certificates for all changed IR's
	for sysID, ir := range x.changes {
		// get certificate for change
		var utCert *types.UnicityTreeCertificate
		utCert, err = ut.GetCertificate(sysID)
		if err != nil {
			return nil, err
		}
		cr := &certification.CertificationResponse{
			Partition: utCert.SystemIdentifier,
			UC: types.UnicityCertificate{
				Version:     1,
				InputRecord: ir,
				UnicityTreeCertificate: &types.UnicityTreeCertificate{Version: 1,
					SystemIdentifier:         utCert.SystemIdentifier,
					HashSteps:                utCert.HashSteps,
					PartitionDescriptionHash: utCert.PartitionDescriptionHash,
				},
				UnicitySeal: uSeal,
			},
		}
		// verify certificate
		if err = cr.UC.Verify(x.trustBase, x.params.HashAlgorithm, utCert.SystemIdentifier, utCert.PartitionDescriptionHash); err != nil {
			// should never happen.
			return nil, fmt.Errorf("error invalid generated unicity certificate: %w", err)
		}
		x.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("NewStateStore unicity certificate for partition %s is", sysID), logger.Data(cr))
		certs = append(certs, cr)
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
	x.changes = make(map[types.SystemID]*types.InputRecord)
	return certs, nil
}
