package abdrc

import (
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus"
	"github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/storage"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/rootchain/partitions"
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	State interface {
		GetCertificates() map[protocol.SystemIdentifier]*types.UnicityCertificate
		IsChangeInProgress(id protocol.SystemIdentifier) bool
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

func t2TimeoutToRootRounds(t2Timeout uint32, blockRate time.Duration) uint64 {
	return uint64((time.Duration(t2Timeout)*time.Millisecond)/blockRate) + 1
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
	// Remember all partitions that have changes in the current proposal and apply changes
	sysID := protocol.SystemIdentifier(irChReq.SystemIdentifier)
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	if x.state.IsChangeInProgress(sysID) {
		return nil, fmt.Errorf("add state failed: partition %X has pending changes in pipeline", sysID.Bytes())
	}
	ucs := x.state.GetCertificates()
	// verify certification Request
	luc, found := ucs[sysID]
	if !found {
		return nil, fmt.Errorf("ir change request verification error, partition %X last certificate not found", sysID)
	}
	if round < luc.UnicitySeal.RootChainRoundNumber {
		return nil, fmt.Errorf("error, current round %v is in the past, luc round %v", round, luc.UnicitySeal.RootChainRoundNumber)
	}

	// Find if the SystemIdentifier is known by partition store
	sysDesRecord, tb, err := x.partitions.GetInfo(sysID)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: unknown partition %X", sysID.Bytes())
	}
	// verify request
	inputRecord, err := irChReq.Verify(tb, luc, round, t2TimeoutToRootRounds(sysDesRecord.T2Timeout, x.params.BlockRate/2))
	if err != nil {
		return nil, fmt.Errorf("invalid payload: partition %X certification request verifiaction failed %w", sysID.Bytes(), err)
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

func (x *PartitionTimeoutGenerator) GetT2Timeouts(currentRound uint64) ([]protocol.SystemIdentifier, error) {
	ucs := x.state.GetCertificates()
	timeoutIds := make([]protocol.SystemIdentifier, 0, len(ucs))
	var err error
	for id, cert := range ucs {
		if x.state.IsChangeInProgress(id) {
			continue
		}
		sysDesc, _, getErr := x.partitions.GetInfo(id)
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
