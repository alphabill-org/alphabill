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
		ShardInfo(partition types.PartitionID, shard types.ShardID) *storage.ShardInfo
		GetCertificates() []*types.UnicityCertificate
		IsChangeInProgress(id types.PartitionID, shard types.ShardID) *types.InputRecord
	}

	IRChangeReqVerifier struct {
		params *Parameters
		state  State
	}

	PartitionTimeoutGenerator struct {
		blockRate time.Duration
		state     State
	}
)

func NewIRChangeReqVerifier(c *Parameters, sMonitor State) (*IRChangeReqVerifier, error) {
	if sMonitor == nil {
		return nil, errors.New("state monitor is nil")
	}
	if c == nil {
		return nil, errors.New("consensus params is nil")
	}
	return &IRChangeReqVerifier{
		params: c,
		state:  sMonitor,
	}, nil
}

func (x *IRChangeReqVerifier) VerifyIRChangeReq(rootRound uint64, irChReq *drctypes.IRChangeReq) (*types.InputRecord, error) {
	if irChReq == nil {
		return nil, fmt.Errorf("IR change request is nil")
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest.
	// This gets the shardInfo from committed round (for which there is UC), and irChReq should build on that.
	si := x.state.ShardInfo(irChReq.Partition, irChReq.Shard)
	if si == nil {
		// There shouldn't be an IR change request for a shard with no committed state
		return nil, fmt.Errorf("missing shard info for partition %d shard %s", irChReq.Partition, irChReq.Shard.String())
	}

	// verify request
	luc := si.LastCR.UC
	inputRecord, err := irChReq.Verify(si, &luc, rootRound, t2TimeoutToRootRounds(si.T2Timeout, x.params.BlockRate/2))
	if err != nil {
		return nil, fmt.Errorf("certification request verification failed: %w", err)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	if ir := x.state.IsChangeInProgress(irChReq.Partition, irChReq.Shard); ir != nil {
		if b, err := types.EqualIR(inputRecord, ir); b || err != nil {
			if err != nil {
				return nil, fmt.Errorf("comparing input records: %w", err)
			}
			return nil, ErrDuplicateChangeReq
		}
		return nil, fmt.Errorf("shard %s-%s has pending changes in pipeline", irChReq.Partition, irChReq.Shard)
	}
	// check - should never happen, somehow the root node round must have been reset
	if rootRound < luc.UnicitySeal.RootChainRoundNumber {
		return nil, fmt.Errorf("current round %v is in the past, LUC round %v", rootRound, luc.UnicitySeal.RootChainRoundNumber)
	}
	return inputRecord, nil
}

func NewLucBasedT2TimeoutGenerator(c *Parameters, sMonitor State) (*PartitionTimeoutGenerator, error) {
	if sMonitor == nil {
		return nil, errors.New("state monitor is nil")
	}
	if c == nil {
		return nil, errors.New("consensus params is nil")
	}
	return &PartitionTimeoutGenerator{
		blockRate: c.BlockRate,
		state:     sMonitor,
	}, nil
}

func (x *PartitionTimeoutGenerator) GetT2Timeouts(currentRound uint64) ([]*types.UnicityCertificate, error) {
	// Only activated shards with an UC can time out. New shards are activated by adding their ShardInfo and
	// an empty IR to the ExecutedBlock in the activation root round. Once the block gets committed, they
	// get their first UC and can start timing out.
	ucs := x.state.GetCertificates()

	timedOutShards := make([]*types.UnicityCertificate, 0, len(ucs))
	for _, uc := range ucs {
		// do not create T2 timeout requests if shard has a change already in pipeline
		if x.state.IsChangeInProgress(uc.GetPartitionID(), uc.GetShardID()) != nil {
			continue
		}

		si := x.state.ShardInfo(uc.GetPartitionID(), uc.GetShardID())
		lastRootRound := uc.GetRootRoundNumber()
		if currentRound-lastRootRound >= t2TimeoutToRootRounds(si.T2Timeout, x.blockRate/2) {
			timedOutShards = append(timedOutShards, uc)
		}
	}
	return timedOutShards, nil
}

func t2TimeoutToRootRounds(t2Timeout time.Duration, blockRate time.Duration) uint64 {
	return uint64(t2Timeout/blockRate) + 1
}
