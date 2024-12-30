package consensus

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	IRChangeVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *drctypes.IRChangeReq) (*storage.InputData, error)
	}

	irChange struct {
		InputRecord *types.InputRecord
		Reason      drctypes.IRChangeReason
		Req         *drctypes.IRChangeReq
	}

	IrReqBuffer struct {
		irChgReqBuffer map[partitionShard]*irChange
		blockRate      time.Duration
		state          State
		getPartitions  RoundPartitionsFn
		log            *slog.Logger
	}

	RoundPartitionsFn func(rootRound uint64) ([]*types.PartitionDescriptionRecord, error)
)

func NewIrReqBuffer(state State, getPartitions RoundPartitionsFn, blockRate time.Duration, log *slog.Logger) *IrReqBuffer {
	return &IrReqBuffer{
		irChgReqBuffer: make(map[partitionShard]*irChange),
		state:          state,
		getPartitions:  getPartitions,
		blockRate:      blockRate,
		log:            log,
	}
}

// Add validates incoming IR change request and buffers valid requests. If for any reason the IR request is found not
// valid, reason is logged, error is returned and request is ignored.
func (x *IrReqBuffer) Add(round uint64, irChReq *drctypes.IRChangeReq, ver IRChangeVerifier) error {
	if irChReq == nil {
		return fmt.Errorf("ir change request is nil")
	}
	// special case, timeout cannot be requested, it can only be added to a block by the leader
	if irChReq.CertReason == drctypes.T2Timeout {
		return fmt.Errorf("invalid ir change request, timeout can only be proposed by leader issuing a new block")
	}
	irData, err := ver.VerifyIRChangeReq(round, irChReq)
	if err != nil {
		return fmt.Errorf("invalid IR Change Request: %w", err)
	}
	partitionID := irChReq.Partition
	key := partitionShard{irChReq.Partition, irChReq.Shard.Key()}
	// verify and extract proposed IR, NB! in this case we set the age to 0 as
	// currently no request can be received to request timeout
	newIrChReq := &irChange{InputRecord: irData.IR, Reason: irChReq.CertReason, Req: irChReq}
	if irChangeReq, found := x.irChgReqBuffer[key]; found {
		if irChangeReq.Reason != newIrChReq.Reason {
			return fmt.Errorf("equivocating request for partition %s, reason has changed", partitionID)
		}
		if b, err := types.EqualIR(irChangeReq.InputRecord, newIrChReq.InputRecord); b || err != nil {
			if err != nil {
				return fmt.Errorf("failed to compare IRs, %w", err)
			}
			// duplicate already stored
			x.log.Debug("duplicate IR change request, ignored", logger.Shard(partitionID, irChReq.Shard))
			return nil
		}
		// At this point it is not possible to cast blame, so just return error and ignore
		return fmt.Errorf("equivocating request for partition %s", partitionID)
	}
	// Insert first valid request received and compare the others received against it
	x.irChgReqBuffer[key] = newIrChReq
	return nil
}

/*
isChangeInBuffer returns true if there is a request for IR change from the shard
in the buffer
*/
func (x *IrReqBuffer) isChangeInBuffer(partition types.PartitionID, shard types.ShardID) bool {
	_, found := x.irChgReqBuffer[partitionShard{partition, shard.Key()}]
	return found
}

func (x *IrReqBuffer) GeneratePayload(ctx context.Context, round uint64) (*drctypes.Payload, error) {
	partitions, err := x.getPartitions(round)
	if err != nil {
		return nil, fmt.Errorf("loading PDRs of the round %d: %w", round, err)
	}
	payload := &drctypes.Payload{}
	for _, pdr := range partitions {
		timeoutRounds := t2TimeoutToRootRounds(pdr.T2Timeout, x.blockRate/2)
		partitionID := pdr.PartitionID
		for shardID := range pdr.Shards.All() {
			if x.state.IsChangeInProgress(partitionID, shardID) != nil {
				// if there is a pending block with the shard in progress then do
				// not propose a change before pending one has been certified
				x.log.DebugContext(ctx, fmt.Sprintf("shard %s - %s not considered for payload, pending change in pipeline", partitionID, shardID), logger.Shard(partitionID, shardID))
				continue
			}

			if req, ok := x.irChgReqBuffer[partitionShard{partitionID, shardID.Key()}]; ok {
				payload.Requests = append(payload.Requests, req.Req)
				continue
			}

			// no change ready for certification, should we generate timeout for the shard?
			si, err := x.state.ShardInfo(partitionID, shardID)
			if err != nil {
				return nil, fmt.Errorf("load shard %s-%s info: %w", partitionID, shardID, err)
			}
			if roundsPassed := round - si.LastCR.UC.GetRootRoundNumber(); roundsPassed >= timeoutRounds {
				payload.Requests = append(payload.Requests, &drctypes.IRChangeReq{
					Partition:  partitionID,
					Shard:      shardID,
					CertReason: drctypes.T2Timeout,
				})
				x.log.DebugContext(ctx, fmt.Sprintf("shard %s - %s timeout IRCR generated", partitionID, shardID), logger.Shard(partitionID, shardID))
			}
		}
	}
	// clear the buffer once payload is done
	clear(x.irChgReqBuffer)
	return payload, nil
}
