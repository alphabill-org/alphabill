package consensus

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

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

var ErrDuplicateChangeReq = errors.New("duplicate ir change request")

func t2TimeoutToRootRounds(t2Timeout time.Duration, blockRate time.Duration) uint64 {
	return uint64(t2Timeout/blockRate) + 1
}

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
	pg, err := x.orchestration.ShardConfig(irChReq.Partition, irChReq.Shard, si.Epoch)
	if err != nil {
		return nil, fmt.Errorf("acquiring shard config: %w", err)
	}
	luc := si.LastCR.UC
	// verify request
	inputRecord, err := irChReq.Verify(si, &luc, round, t2TimeoutToRootRounds(pg.PartitionDescription.T2Timeout, x.params.BlockRate/2))
	if err != nil {
		return nil, fmt.Errorf("certification request verification failed: %w", err)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	if ir := x.state.IsChangeInProgress(irChReq.Partition, irChReq.Shard); ir != nil {
		// If the same change is already in progress then report duplicate error
		if types.EqualIR(inputRecord, ir) {
			return nil, ErrDuplicateChangeReq
		}
		return nil, fmt.Errorf("add state failed: partition %s has pending changes in pipeline", irChReq.Partition)
	}
	// check - should never happen, somehow the root node round must have been reset
	if round < luc.UnicitySeal.RootChainRoundNumber {
		return nil, fmt.Errorf("current round %v is in the past, LUC round %v", round, luc.UnicitySeal.RootChainRoundNumber)
	}
	return &storage.InputData{
			Partition: irChReq.Partition,
			Shard:     irChReq.Shard,
			IR:        inputRecord,
			PDRHash:   pg.PartitionDescription.Hash(x.params.HashAlgorithm),
		},
		nil
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
	configs, err := x.orchestration.RoundPartitions(currentRound)
	if err != nil {
		return nil, err
	}
	timeoutIds := make([]types.PartitionID, 0, len(configs))
	for _, partition := range configs {
		partitionID := partition.PartitionDescription.PartitionIdentifier
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
		US := si.LastCR.UC.UnicitySeal
		if currentRound-US.RootChainRoundNumber >= t2TimeoutToRootRounds(partition.PartitionDescription.T2Timeout, x.blockRate/2) {
			timeoutIds = append(timeoutIds, partitionID)
		}
	}
	return timeoutIds, err
}
