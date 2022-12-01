package distributed

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/request_store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

const ErrStr = "state store is nil"

type (
	ChangeSet map[protocol.SystemIdentifier]struct{}

	CertInputData struct {
		Ir      *certificates.InputRecord
		SysDesc *genesis.SystemDescriptionRecord
	}

	StateLedgerEntry struct {
		State   store.RootState
		Changed ChangeSet
	}

	StateLedger struct {
		stateLedgerMap map[uint64]*StateLedgerEntry
		HighQC         *atomic_broadcast.QuorumCert // highest QC seen
		HighCommitQC   *atomic_broadcast.QuorumCert // highest QC serving as commit certificate
		stateStore     StateStore                   // certified and committed states
		hashAlgorithm  gocrypto.Hash                // hash algorithm
	}
)

func (s *ChangeSet) add(sysId protocol.SystemIdentifier) {
	(*s)[sysId] = struct{}{}
}

func (s *ChangeSet) clear() {
	*s = make(ChangeSet)
}

func (s *ChangeSet) contains(sysId protocol.SystemIdentifier) bool {
	_, ok := (*s)[sysId]
	return ok
}

func NewStateLedger(stateStore StateStore, hash gocrypto.Hash) (*StateLedger, error) {
	if stateStore == nil {
		return nil, errors.New(ErrStr)
	}
	_, err := stateStore.Get()
	if err != nil {
		return nil, err
	}
	// populate pending state with existing state
	return &StateLedger{
		stateLedgerMap: make(map[uint64]*StateLedgerEntry),
		stateStore:     stateStore,
		hashAlgorithm:  hash,
	}, nil
}

func (s *StateLedger) ProcessQc(qc *atomic_broadcast.QuorumCert) {
	if qc == nil {
		return
	}
	// If the QC commits a state
	if len(qc.LedgerCommitInfo.CommitStateId) != 0 {
		state, found := s.stateLedgerMap[qc.VoteInfo.ParentRound]
		if found {
			// Commit pending state if it has the same state hash
			if bytes.Equal(state.State.LatestRootHash, qc.LedgerCommitInfo.CommitStateId) {
				//todo: create new UnicitySeal structure and store to DB
				// return state for sending result to partition manager
				// delete ledger entry
				delete(s.stateLedgerMap, qc.VoteInfo.ParentRound)
			}
		}
		s.HighCommitQC = qc
	}
	s.HighQC = qc
}

func (s *StateLedger) ProcessTc(tc *atomic_broadcast.TimeoutCert) {
	if tc == nil {
		return
	}
	// clear map, the states will never be committed anyway
	s.stateLedgerMap = make(map[uint64]*StateLedgerEntry)
}

func (s *StateLedger) ExecuteProposalPayload(round uint64, req *atomic_broadcast.Payload, partitions PartitionStore) error {
	if req == nil {
		return errors.New("no payload")
	}
	// Certify input, everything needs to be verified again as if received from partition node, since we cannot
	// trust the proposer is honest
	requests := request_store.NewCertificationRequestStore()
	// Get previous state, is the previous round pending
	previousState, found := s.stateLedgerMap[round-1]
	// If there is nothing pending, then just read the latest stored state
	if !found {
		// Nothing concerning this round is pending certification, get state from store
		state, err := s.stateStore.Get()
		if err != nil {
			return err
		}
		previousState = &StateLedgerEntry{
			State:   state,
			Changed: make(map[protocol.SystemIdentifier]struct{}),
		}
	}
	inputData := make(map[protocol.SystemIdentifier]*CertInputData)
	// Init with previous state
	for id, luc := range previousState.State.Certificates {
		sysDesc, err := partitions.GetSystemDescription(id)
		if err != nil {
			return fmt.Errorf("erros reading partition %X info", id.Bytes())
		}
		inputData[id].Ir = luc.InputRecord
		inputData[id].SysDesc = sysDesc
	}
	// Remember all partitions that have changes in the current proposal and apply changes
	changed := ChangeSet{}
	for _, certReq := range req.Requests {
		systemId := protocol.SystemIdentifier(certReq.SystemIdentifier)
		if previousState.Changed.contains(systemId) {
			// trying to certify changes in consecutive rounds is not possible, this is a malicious
			return fmt.Errorf("invalid certification request, changes for partition %X in previous round %v",
				systemId.Bytes(), round-1)
		}
		// Find if the SystemIdentifier is known by partition store
		sysDesc, err := partitions.GetSystemDescription(systemId)
		if err != nil {
			logger.Warning("Payload contains unknown partition %X, ignoring", systemId)
			// Still process the rest and vote, it is better to vote differently than send nothing
			continue
		}
		for _, req := range certReq.Requests {
			if systemId != protocol.SystemIdentifier(req.SystemIdentifier) {
				return errors.New("invalid payload")
			}
			if err := requests.Add(req); err != nil {
				return fmt.Errorf("equivocating requets in payload from partition %X", systemId.Bytes())
			}
		}
		nodeCnt, err := partitions.GetNofNodesInPartition(systemId)
		if err != nil {
			return fmt.Errorf("cannot verify quorum for certification request: %w", err)
		}
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
			cert, found := previousState.State.Certificates[systemId]
			if !found {
				return errors.Errorf("missing state for partition id: %X", systemId)
			}
			// verify timeout ok
			lucAgeInRounds := round - cert.UnicitySeal.RootChainRoundNumber
			if lucAgeInRounds*500 < uint64(sysDesc.T2Timeout) {
				logger.Warning("Payload invalid timeout id %X, time from latest UC %v, timeout %v, ignoring",
					systemId, lucAgeInRounds*500, sysDesc.T2Timeout)
				continue
			}
			// copy last input record
			inputRecord = cert.InputRecord
			break
		}
		inputData[systemId] = &CertInputData{Ir: inputRecord, SysDesc: sysDesc}
		changed.add(systemId)
	}

	// Input records are now available, time to calculate new UC's
	newState, err := CreateUnicityCertificates(s.hashAlgorithm, round, previousState.State.LatestRootHash, inputData)
	if err != nil {
		logger.Warning("Failed to generate unicity certificates for round %v", round)
		return err
	}
	s.stateLedgerMap[round] = &StateLedgerEntry{State: *newState, Changed: changed}
	return nil
}

// CreateUnicityCertificates certifies input records and returns state containing changes (new root, round, UCs changed)
func CreateUnicityCertificates(algo gocrypto.Hash, round uint64, prevHash []byte,
	inputData map[protocol.SystemIdentifier]*CertInputData) (*store.RootState, error) {
	data := make([]*unicitytree.Data, len(inputData))
	i := 0
	for key, rec := range inputData {
		data[i] = &unicitytree.Data{
			SystemIdentifier:            key.Bytes(),
			InputRecord:                 rec.Ir,
			SystemDescriptionRecordHash: rec.SysDesc.Hash(algo),
		}
		i++
	}
	logger.Debug("Input records are:")
	for key, ir := range inputData {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("IR for partition %X is:", key.Bytes()), ir)
	}
	ut, err := unicitytree.New(algo.New(), data)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	logger.Info("Round: %v, root hash: %X, creating %v unicity certificates", round, rootHash, len(inputData))
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, d := range data {
		cert, err := ut.GetCertificate(d.SystemIdentifier)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
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
		certs[protocol.SystemIdentifier(d.SystemIdentifier)] = certificate
		util.WriteDebugJsonLog(logger, fmt.Sprintf("New unicity certificate for partition %X is", d.SystemIdentifier), certificate)
	}
	// Return state delta
	return &store.RootState{
		LatestRound:    round,
		LatestRootHash: prevHash,
		Certificates:   certs}, nil
}

func (s *StateLedger) GetRoundState(round uint64) (*StateLedgerEntry, error) {
	entry, found := s.stateLedgerMap[round]
	if !found {
		// Nothing concerning this round is pending certification, get state from store
		state, err := s.stateStore.Get()
		if err != nil {
			return nil, err
		}
		if state.LatestRound != round {
			return nil, fmt.Errorf("round not found")
		}
		return &StateLedgerEntry{
			State:   state,
			Changed: make(map[protocol.SystemIdentifier]struct{}),
		}, nil
	}
	return entry, nil
}
