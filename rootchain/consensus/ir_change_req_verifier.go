package consensus

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

var ErrDuplicateChangeReq = errors.New("duplicate ir change request")

type (
	State interface {
		ShardInfo(partition types.PartitionID, shard types.ShardID) (*storage.ShardInfo, error)
		GetCertificates() []*types.UnicityCertificate
		IsChangeInProgress(id types.PartitionID, shard types.ShardID) *types.InputRecord
	}

	IRChangeReqVerifier struct {
		params        *Parameters
		state         State
		orchestration Orchestration
	}

	PartitionTimeoutGenerator struct {
		blockRate     time.Duration
		state         State
		orchestration Orchestration
	}
)

func NewIRChangeReqVerifier(c *Parameters, orchestration Orchestration, sMonitor State) (*IRChangeReqVerifier, error) {
	if sMonitor == nil {
		return nil, errors.New("state monitor is nil")
	}
	if orchestration == nil {
		return nil, errors.New("orchestration is nil")
	}
	if c == nil {
		return nil, errors.New("consensus params is nil")
	}
	return &IRChangeReqVerifier{
		params:        c,
		orchestration: orchestration,
		state:         sMonitor,
	}, nil
}

func (x *IRChangeReqVerifier) VerifyIRChangeReq(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error) {
	if irChReq == nil {
		return nil, fmt.Errorf("IR change request is nil")
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest
	si, err := x.state.ShardInfo(irChReq.Partition, irChReq.Shard)
	if err != nil {
		return nil, fmt.Errorf("acquiring shard info: %w", err)
	}

	epoch, err := x.orchestration.ShardEpoch(irChReq.Partition, irChReq.Shard, round)
	if err != nil {
		return nil, fmt.Errorf("querying shard epoch: %w", err)
	}
	pdr, err := x.orchestration.PartitionDescription(irChReq.Partition, epoch)
	if err != nil {
		return nil, fmt.Errorf("acquiring partition genesis: %w", err)
	}
	// verify request
	luc := si.LastCR.UC
	inputRecord, err := irChReq.Verify(si, &luc, round, t2TimeoutToRootRounds(pdr.T2Timeout, x.params.BlockRate/2))
	if err != nil {
		return nil, fmt.Errorf("certification request verification failed: %w", err)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	if ir := x.state.IsChangeInProgress(irChReq.Partition, irChReq.Shard); ir != nil {
		// If the same change is already in progress then report duplicate error
		if b, err := types.EqualIR(inputRecord, ir); b || err != nil {
			if err != nil {
				return nil, fmt.Errorf("comparing input records: %w", err)
			}
			return nil, ErrDuplicateChangeReq
		}
		return nil, fmt.Errorf("add state failed: partition %s has pending changes in pipeline", irChReq.Partition)
	}
	// check - should never happen, somehow the root node round must have been reset
	if round < luc.UnicitySeal.RootChainRoundNumber {
		return nil, fmt.Errorf("current round %v is in the past, LUC round %v", round, luc.UnicitySeal.RootChainRoundNumber)
	}
	pdrHash, err := pdr.Hash(x.params.HashAlgorithm)
	if err != nil {
		return nil, fmt.Errorf("hashing partition description: %w", err)
	}
	return &storage.InputData{
		Partition: irChReq.Partition,
		Shard:     irChReq.Shard,
		IR:        inputRecord,
		PDRHash:   pdrHash,
	}, nil
}

func NewLucBasedT2TimeoutGenerator(c *Parameters, orchestration Orchestration, sMonitor State) (*PartitionTimeoutGenerator, error) {
	if sMonitor == nil {
		return nil, errors.New("state monitor is nil")
	}
	if orchestration == nil {
		return nil, errors.New("orchestration is nil")
	}
	if c == nil {
		return nil, errors.New("consensus params is nil")
	}
	return &PartitionTimeoutGenerator{
		blockRate:     c.BlockRate,
		orchestration: orchestration,
		state:         sMonitor,
	}, nil
}

func (x *PartitionTimeoutGenerator) GetT2Timeouts(currentRound uint64) (_ []types.PartitionID, retErr error) {
	pdrs, err := x.orchestration.RoundPartitions(currentRound)
	if err != nil {
		return nil, fmt.Errorf("loading PDRs of the round %d: %w", currentRound, err)
	}
	timeoutIds := make([]types.PartitionID, 0, len(pdrs))
	for _, partition := range pdrs {
		partitionID := partition.PartitionIdentifier
		// do not create T2 timeout requests if partition has a change already in pipeline
		if ir := x.state.IsChangeInProgress(partitionID, types.ShardID{}); ir != nil {
			continue
		}
		si, err := x.state.ShardInfo(partitionID, types.ShardID{})
		if err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("read shard %s info: %w", partitionID, err))
			// still try to check the rest of the partitions
			continue
		}
		lastRootRound := si.LastCR.UC.UnicitySeal.RootChainRoundNumber
		if currentRound-lastRootRound >= t2TimeoutToRootRounds(partition.T2Timeout, x.blockRate/2) {
			timeoutIds = append(timeoutIds, partitionID)
		}
	}
	return timeoutIds, retErr
}

func t2TimeoutToRootRounds(t2Timeout time.Duration, blockRate time.Duration) uint64 {
	return uint64(t2Timeout/blockRate) + 1
}
