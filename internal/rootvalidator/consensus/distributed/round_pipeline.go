package distributed

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	StateEntry struct {
		State   *store.RootState
		Changed map[protocol.SystemIdentifier]*certificates.InputRecord
		Qc      *atomic_broadcast.QuorumCert
	}

	RoundPipeline struct {
		hashAlgorithm gocrypto.Hash                                           // hash algorithm
		partitions    PartitionStore                                          // partition store
		ir            map[protocol.SystemIdentifier]*certificates.InputRecord // currently valid input records
		execStateID   []byte
		inProgress    map[protocol.SystemIdentifier]struct{}
		statePipeline map[uint64]*StateEntry
		highQC        *atomic_broadcast.QuorumCert // highest QC seen
	}
)

func NewRoundPipeline(hash gocrypto.Hash, persistedState store.RootState, partitionStore PartitionStore) *RoundPipeline {
	//init IR map
	var hQC *atomic_broadcast.QuorumCert = nil
	inputRecords := make(map[protocol.SystemIdentifier]*certificates.InputRecord)
	for id, cert := range persistedState.Certificates {
		// initiate the highest quorum certificate from persisted state
		if hQC == nil && bytes.Equal(cert.UnicitySeal.CommitInfo.RootHash, persistedState.LatestRootHash) {
			hQC = &atomic_broadcast.QuorumCert{
				VoteInfo:         cert.UnicitySeal.RootRoundInfo,
				LedgerCommitInfo: cert.UnicitySeal.CommitInfo,
				Signatures:       cert.UnicitySeal.Signatures,
			}
		}
		//remember highest quorum certificate
		inputRecords[id] = cert.InputRecord
	}
	// initiate from last persisted/committed state
	return &RoundPipeline{
		hashAlgorithm: hash,
		partitions:    partitionStore,
		ir:            inputRecords,
		execStateID:   persistedState.LatestRootHash,
		inProgress:    make(map[protocol.SystemIdentifier]struct{}),
		statePipeline: make(map[uint64]*StateEntry),
		highQC:        hQC,
	}
}

func (x *RoundPipeline) Reset(persistedState store.RootState) {
	// clear map, the states will never be committed anyway
	x.statePipeline = make(map[uint64]*StateEntry)
	x.inProgress = make(map[protocol.SystemIdentifier]struct{})
	x.ir = make(map[protocol.SystemIdentifier]*certificates.InputRecord)
	for id, cert := range persistedState.Certificates {
		x.ir[id] = cert.InputRecord
	}
	x.execStateID = persistedState.LatestRootHash
	return
}

func (x *RoundPipeline) IsChangeInPipeline(sysId protocol.SystemIdentifier) bool {
	_, f := x.inProgress[sysId]
	return f
}

func (x *RoundPipeline) GetExecStateId() []byte {
	return x.execStateID
}

func (x *RoundPipeline) removeCompleted(round uint64, changes map[protocol.SystemIdentifier]*certificates.InputRecord) {
	// remove committed changes from inProgress
	for ch := range changes {
		delete(x.inProgress, ch)
	}
	// remove completed round from pipeline
	delete(x.statePipeline, round)
}

func (x *RoundPipeline) Update(qc *atomic_broadcast.QuorumCert) *store.RootState {
	if qc == nil {
		return nil
	}
	// remember the highest QC seen so far
	x.highQC = qc
	// Add qc to pending state (needed for recovery)
	state, found := x.statePipeline[qc.VoteInfo.RoundNumber]
	if found {
		state.Qc = qc
	}
	// This QC does not serve as commit QC
	if qc.LedgerCommitInfo.RootHash == nil {
		return nil
	}
	// If the QC commits a state
	// Add qc to pending state (needed for recovery)
	state, found = x.statePipeline[qc.VoteInfo.ParentRoundNumber]
	if found && bytes.Equal(state.State.LatestRootHash, qc.LedgerCommitInfo.RootHash) {
		// Commit pending state if it has the same root hash as committed state
		// create UnicitySeal for pending certificates
		uSeal := &certificates.UnicitySeal{
			RootRoundInfo: qc.VoteInfo,
			CommitInfo:    qc.LedgerCommitInfo,
			Signatures:    qc.Signatures,
		}
		commitState := state.State
		// append Seal to all certificates
		for _, cert := range commitState.Certificates {
			cert.UnicitySeal = uSeal
		}
		// remove committed state from pipeline
		x.removeCompleted(qc.VoteInfo.ParentRoundNumber, state.Changed)
		return commitState
	}
	return nil
}

func (x *RoundPipeline) Add(round uint64, changes map[protocol.SystemIdentifier]*certificates.InputRecord) ([]byte, error) {
	// verify that state for the round does not exist yet
	if _, found := x.statePipeline[round]; found {
		return nil, fmt.Errorf("add state failed: state for round %v already in pipeline", round)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	for sysID := range changes {
		if _, f := x.inProgress[sysID]; f {
			return nil, fmt.Errorf("add state failed: partition %X has pending changes in pipeline", sysID.Bytes())
		}
	}
	if len(changes) == 0 {
		logger.Debug("Round %v executing proposal, no changes to input records", round)
	} else {
		logger.Debug("Round %v executing proposal, changed input records are:", round)
	}
	// apply changes
	for id, ch := range changes {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("partition %X IR:", id), ch)
		x.ir[id] = ch
		x.inProgress[id] = struct{}{}
	}
	utData := make([]*unicitytree.Data, 0, len(x.ir))
	for id, ir := range x.ir {
		partInfo, err := x.partitions.GetPartitionInfo(id)
		if err != nil {
			return nil, err
		}
		sdrh := partInfo.SystemDescription.Hash(x.hashAlgorithm)
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            partInfo.SystemDescription.SystemIdentifier,
			InputRecord:                 ir,
			SystemDescriptionRecordHash: sdrh,
		})
	}
	ut, err := unicitytree.New(x.hashAlgorithm.New(), utData)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for sysID, ir := range changes {
		utCert, err := ut.GetCertificate(sysID.Bytes())
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		certificate := &certificates.UnicityCertificate{
			InputRecord: ir,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
		}
		certs[sysID] = certificate
	}
	x.execStateID = rootHash
	x.statePipeline[round] = &StateEntry{State: &store.RootState{
		LatestRound:    round,
		LatestRootHash: rootHash,
		Certificates:   certs,
	}, Changed: changes, Qc: nil}
	return rootHash, nil
}

func (x *RoundPipeline) GetHighQc() *atomic_broadcast.QuorumCert {
	return x.highQC
}
