package distributed

import (
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus/distributed/storage"
)

type (
	State interface {
		GetCertificates() map[protocol.SystemIdentifier]*certificates.UnicityCertificate
		IsChangeInProgress(id protocol.SystemIdentifier) bool
	}

	IRChangeReqVerifier struct {
		params        *consensus.Parameters
		state         State
		partitionInfo PartitionStore
	}

	PartitionTimeoutGenerator struct {
		params        *consensus.Parameters
		state         State
		partitionInfo PartitionStore
	}
)

func NewIRChangeReqVerifier(c *consensus.Parameters, pInfo PartitionStore, sMonitor State) (*IRChangeReqVerifier, error) {
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
		params:        c,
		partitionInfo: pInfo,
		state:         sMonitor,
	}, nil
}

func (x *IRChangeReqVerifier) getLastUnicityCertificate(id protocol.SystemIdentifier) (*certificates.UnicityCertificate, error) {
	ucs := x.state.GetCertificates()
	// verify certification Request
	if luc, found := ucs[id]; found == true {
		return luc, nil
	}
	return nil, errors.Errorf("partition %X last certificate not found", id)
}

func (x *IRChangeReqVerifier) VerifyIRChangeReq(round uint64, irChReq *atomic_broadcast.IRChangeReqMsg) (*storage.InputData, error) {
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
	luc, err := x.getLastUnicityCertificate(sysID)
	if err != nil {
		return nil, fmt.Errorf("ir change request verification error, %w", err)
	}
	if round < luc.UnicitySeal.RootRoundInfo.RoundNumber {
		return nil, fmt.Errorf("error, current round %v is in the past, luc round %v", round, luc.UnicitySeal.RootRoundInfo.RoundNumber)
	}

	// Find if the SystemIdentifier is known by partition store
	partitionInfo, err := x.partitionInfo.Info(sysID)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: unknown partition %X", sysID.Bytes())
	}
	lucAgeInRounds := time.Duration(round-luc.UnicitySeal.RootRoundInfo.RoundNumber) * x.params.BlockRateMs
	// verify request
	inputRecord, err := irChReq.Verify(partitionInfo, luc, lucAgeInRounds)
	if err != nil {
		return nil, fmt.Errorf("invalid payload: partition %X certification request verifiaction failed %w", sysID.Bytes(), err)
	}
	return &storage.InputData{SysID: irChReq.SystemIdentifier, IR: inputRecord, Sdrh: partitionInfo.SystemDescription.Hash(x.params.HashAlgorithm)}, nil
}

func NewLucBasedT2TimeoutGenerator(c *consensus.Parameters, pInfo PartitionStore, sMonitor State) (*PartitionTimeoutGenerator, error) {
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
		params:        c,
		partitionInfo: pInfo,
		state:         sMonitor,
	}, nil
}

func (x *PartitionTimeoutGenerator) GetT2Timeouts(currenRound uint64) []protocol.SystemIdentifier {
	ucs := x.state.GetCertificates()
	timeoutIds := make([]protocol.SystemIdentifier, 0, len(ucs))
	for id, cert := range ucs {
		if x.state.IsChangeInProgress(id) {
			continue
		}
		partInfo, err := x.partitionInfo.Info(id)
		if err != nil {
			logger.Error("round %v failed to generate timeout request for partition %v, %w", id.Bytes(), err)
			// still try to compose a payload, better than nothing
			continue
		}
		if time.Duration(currenRound-cert.UnicitySeal.RootRoundInfo.RoundNumber)*x.params.BlockRateMs >=
			time.Duration(partInfo.SystemDescription.T2Timeout)*time.Millisecond {
			timeoutIds = append(timeoutIds, id)
		}
	}
	return timeoutIds
}
