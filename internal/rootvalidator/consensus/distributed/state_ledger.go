package distributed

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

const ErrStr = "state store is nil"

type StateLedger struct {
	proposedState        *store.RootState             // last proposed state
	pendingCertification *store.RootState             // state waiting for second QC
	HighQC               *atomic_broadcast.QuorumCert // highest QC seen
	HighCommitQC         *atomic_broadcast.QuorumCert // highest QC serving as commit certificate
	stateStore           StateStore                   // certified and committed states
	partitionStore       PartitionStore               // partition store
	hashAlgorithm        gocrypto.Hash                // hash algorithm
}

func (p *StateLedger) ProposedState() *store.RootState {
	return p.proposedState
}

func NewStateLedger(stateStore StateStore, partStore PartitionStore, hash gocrypto.Hash) (*StateLedger, error) {
	if stateStore == nil {
		return nil, errors.New(ErrStr)
	}
	_, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	// populate pending state with existing state
	return &StateLedger{
		proposedState:        nil,
		pendingCertification: nil,
		stateStore:           stateStore,
		partitionStore:       partStore,
		hashAlgorithm:        hash,
	}, nil
}

func (p *StateLedger) ProcessQc(qc *atomic_broadcast.QuorumCert) {
	if qc == nil {
		return
	}
	// If the QC commits a state
	if len(qc.LedgerCommitInfo.CommitStateId) != 0 {
		// Proposed state to pending state
		// Commit pending state if it has the same state hash
		if bytes.Equal(p.pendingCertification.GetStateHash(p.hashAlgorithm), qc.LedgerCommitInfo.CommitStateId) {
			//todo: append second QC and commit to DB, but needs the new UnicitySeal structure
		}
		p.HighCommitQC = qc
	}
	p.HighQC = qc
}

func (p *StateLedger) ProcessTc(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	// Reset all and start from last certified state in DB
	p.proposedState = nil
	p.pendingCertification = nil
}

func (p *StateLedger) ExecuteProposalPayload(round uint64, req *atomic_broadcast.Payload) error {
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot
	// trust the proposer is honest
	requests := request_store.NewCertificationRequestStore()
	if req == nil {
		return errors.New("no payload")
	}
	nofInputs := len(req.Requests)
	data := make([]*unicitytree.Data, nofInputs)
	i := 0
	lastState, err := p.stateStore.Get()
	if err != nil {
		return errors.New("error reading state")
	}
	for _, certReq := range req.Requests {
		// todo: verify signature on the request
		// todo: verify that it extends from previous certified state and there is no intermediate change in process
		systemId := protocol.SystemIdentifier(certReq.SystemIdentifier)
		// Find if the id is known
		info, err := p.partitionStore.GetInfo(systemId)
		if err != nil {
			logger.Warning("Payload contains unknown partition %X, ignoring", systemId)
			// Still process the rest and vote, it is better to vote differently than send nothing
			continue
		}
		sdrh := info.SystemDescription.Hash(p.hashAlgorithm)
		for _, req := range certReq.Requests {
			if systemId != protocol.SystemIdentifier(req.SystemIdentifier) {
				return errors.New("invalid payload")
			}
			if err := requests.Add(req); err != nil {

			}
		}
		nodeCnt := len(info.TrustBase)
		inputRecord, consensusPossible := requests.IsConsensusReceived(systemId, nodeCnt)

		// match request type to proof
		switch certReq.CertReason {
		case atomic_broadcast.IRChangeReqMsg_QUORUM:
			if inputRecord == nil {
				return errors.New("invalid certification request proof, expected quorum not achieved")
			}
			break

		case atomic_broadcast.IRChangeReqMsg_QUORUM_NOT_POSSIBLE:
			if inputRecord != nil && consensusPossible == false {
				return errors.New("invalid certification request proof, no quorum achieved")
			}
			break
		case atomic_broadcast.IRChangeReqMsg_T2_TIMEOUT:
			// timout does not carry proof in form of certification requests
			// validate timeout against round number (or timestamp in unicity seal)
			cert, found := lastState.Certificates[systemId]
			if !found {
				return errors.Errorf("missing state for partition id: %X", systemId)
			}
			// verify timeout ok
			lucAgeInRounds := round - cert.UnicitySeal.RootChainRoundNumber
			if lucAgeInRounds*500 < uint64(info.SystemDescription.T2Timeout) {
				logger.Warning("Payload invalid timeout id %X, time from latest UC %v, timeout %v, ignoring",
					systemId, lucAgeInRounds*500, info.SystemDescription.T2Timeout)
				continue
			}
			// copy last input record
			inputRecord = cert.InputRecord
			break
		}
		data[i] = &unicitytree.Data{
			SystemIdentifier:            []byte(systemId),
			InputRecord:                 inputRecord,
			SystemDescriptionRecordHash: sdrh,
		}
		i++
	}
	// prepare new proposed state
	// Extend state from last pending certification
	if p.pendingCertification.IsValid() != nil {
		// If not valid, then either TC has occurred or we have just started
		// copy last certified state from DB
		*p.proposedState = lastState
	} else {
		*p.proposedState = *p.pendingCertification
	}
	// Input records are now available, time to calculate new UC's
	stateUpdate, err := CreateUnicityCertificates(p.hashAlgorithm, round, p.proposedState.LatestRootHash, data)
	if err != nil {
		logger.Warning("Failed to generate unicity certificates for round %v", round)
		return err
	}
	p.proposedState.Update(*stateUpdate)
	return nil
}

// CreateUnicityCertificates certifies input records and returns state containing changes (new root, round, UCs changed)
func CreateUnicityCertificates(algo gocrypto.Hash, round uint64, prevHash []byte, inputData []*unicitytree.Data) (*store.RootState, error) {
	if len(inputData) == 0 {
		// Empty proposal, return empty map, no changes to UC's
		logger.Debug("No input records")
		return &store.RootState{
			LatestRound:    round,
			LatestRootHash: prevHash,
			Certificates:   make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)}, nil
	}
	logger.Debug("Input records are:")
	for _, ir := range inputData {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("IR for partition %X is:", ir.SystemIdentifier), ir)
	}
	ut, err := unicitytree.New(algo.New(), inputData)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	logger.Info("New root hash is %X", rootHash)
	if err != nil {
		logger.Info("Failed to read last state from storage: %v", err.Error())
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	logger.Info("Creating unicity certificates. RoundNr %v, inputRecords: %v", round, len(inputData))

	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, d := range inputData {
		cert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		identifier := protocol.SystemIdentifier(d.SystemIdentifier)
		sdrHash := d.SystemDescriptionRecordHash

		certificate := &certificates.UnicityCertificate{
			InputRecord: d.InputRecord,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      cert.SystemIdentifier,
				SiblingHashes:         cert.SiblingHashes,
				SystemDescriptionHash: sdrHash,
			},
			UnicitySeal: &certificates.UnicitySeal{
				RootChainRoundNumber: round,
				PreviousHash:         prevHash,
				Hash:                 rootHash,
				RoundCreationTime:    util.MakeTimestamp(),
				Signatures:           make(map[string][]byte),
			},
		}
		certs[identifier] = certificate
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)
	}
	// Return state delta
	return &store.RootState{
		LatestRound:    round,
		LatestRootHash: prevHash,
		Certificates:   certs}, nil
}

func (p *StateLedger) GetCurrentStateHash() []byte {
	if err := p.pendingCertification.IsValid(); err == nil {
		return p.pendingCertification.GetStateHash(p.hashAlgorithm)
	} else {
		lastState, err := p.stateStore.Get()
		if err != nil {
			return nil
		}
		return lastState.GetStateHash(p.hashAlgorithm)
	}
}

func (p *StateLedger) GetProposedStateHash() []byte {
	if err := p.proposedState.IsValid(); err == nil {
		return p.proposedState.GetStateHash(p.hashAlgorithm)
	}
	return nil
}

func (p *StateLedger) GetPartitionTrustBase(id protocol.SystemIdentifier) (map[string]crypto.Verifier, error) {
	info, err := p.partitionStore.GetInfo(id)
	if err != nil {
		return nil, err
	}
	return info.TrustBase, nil
}
