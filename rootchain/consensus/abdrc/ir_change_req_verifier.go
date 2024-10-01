package abdrc

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

type (
	State interface {
		GetCertificate(id types.SystemID, round uint64) (*types.UnicityCertificate, error)
		GetCertificates(round uint64) (map[types.SystemID]*types.UnicityCertificate, error)
		IsChangeInProgress(id types.SystemID) *types.InputRecord
	}

	IRChangeReqVerifier struct {
		params     *consensus.Parameters
		state      State
		partitions partitions.PartitionConfiguration
	}

	PartitionTimeoutGenerator struct {
		params     *consensus.Parameters
		state      State
		partitions partitions.PartitionConfiguration
	}
)

var ErrDuplicateChangeReq = errors.New("duplicate ir change request")

func t2TimeoutToRootRounds(t2Timeout time.Duration, blockRate time.Duration) uint64 {
	return uint64(t2Timeout/blockRate) + 1
}

func NewIRChangeReqVerifier(c *consensus.Parameters, pInfo partitions.PartitionConfiguration, sMonitor State) (*IRChangeReqVerifier, error) {
	if sMonitor == nil {
		return nil, errors.New("error state monitor is nil")
	}
	if pInfo == nil {
		return nil, errors.New("error partition store is nil")
	}
	if c == nil {
		return nil, errors.New("error consensus params is nil")
	}
	return &IRChangeReqVerifier{
		params:     c,
		partitions: pInfo,
		state:      sMonitor,
	}, nil
}

func (x *IRChangeReqVerifier) VerifyIRChangeReq(round uint64, irChReq *abtypes.IRChangeReq) (*storage.InputData, error) {
	if irChReq == nil {
		return nil, fmt.Errorf("IR change request is nil")
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot trust the leader is honest
	sysID := irChReq.SystemIdentifier
	// verify certification Request
	luc, err := x.state.GetCertificate(sysID, round)
	if err != nil {
		return nil, fmt.Errorf("reading partition certificate: %w", err)
	}
	// Find if the SystemIdentifier is known by partition store
	sysDesRecord, tb, err := x.partitions.GetInfo(sysID, round)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: unknown partition %s", sysID)
	}
	// verify request
	inputRecord, err := irChReq.Verify(tb, luc, round, t2TimeoutToRootRounds(sysDesRecord.T2Timeout, x.params.BlockRate/2))
	if err != nil {
		return nil, fmt.Errorf("certification request verification failed: %w", err)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	if ir := x.state.IsChangeInProgress(sysID); ir != nil {
		// If the same change is already in progress then report duplicate error
		if types.EqualIR(inputRecord, ir) {
			return nil, ErrDuplicateChangeReq
		}
		return nil, fmt.Errorf("add state failed: partition %s has pending changes in pipeline", sysID)
	}
	// check - should never happen, somehow the root node round must have been reset
	if round < luc.UnicitySeal.RootChainRoundNumber {
		return nil, fmt.Errorf("current round %v is in the past, LUC round %v", round, luc.UnicitySeal.RootChainRoundNumber)
	}
	return &storage.InputData{SysID: irChReq.SystemIdentifier, IR: inputRecord, Sdrh: sysDesRecord.Hash(x.params.HashAlgorithm)}, nil
}

func NewLucBasedT2TimeoutGenerator(c *consensus.Parameters, pInfo partitions.PartitionConfiguration, sMonitor State) (*PartitionTimeoutGenerator, error) {
	if sMonitor == nil {
		return nil, errors.New("error state monitor is nil")
	}
	if pInfo == nil {
		return nil, errors.New("error partition store is nil")
	}
	if c == nil {
		return nil, errors.New("error consensus params is nil")
	}
	return &PartitionTimeoutGenerator{
		params:     c,
		partitions: pInfo,
		state:      sMonitor,
	}, nil
}

func (x *PartitionTimeoutGenerator) GetT2Timeouts(currentRound uint64) ([]types.SystemID, error) {
	ucs, err := x.state.GetCertificates(currentRound)
	if err != nil {
		return []types.SystemID{}, err
	}
	timeoutIds := make([]types.SystemID, 0, len(ucs))
	for id, cert := range ucs {
		// do not create T2 timeout requests if partition has a change already in pipeline
		if ir := x.state.IsChangeInProgress(id); ir != nil {
			continue
		}
		sysDesc, _, getErr := x.partitions.GetInfo(id, currentRound)
		if getErr != nil {
			err = errors.Join(err, fmt.Errorf("read partition system description failed: %w", getErr))
			// still try to check the rest of the partitions
			continue
		}
		if currentRound-cert.UnicitySeal.RootChainRoundNumber >= t2TimeoutToRootRounds(sysDesc.T2Timeout, x.params.BlockRate/2) {
			timeoutIds = append(timeoutIds, id)
		}
	}
	return timeoutIds, err
}
