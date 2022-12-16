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
		HighQC        *atomic_broadcast.QuorumCert                            // highest QC seen
		hashAlgorithm gocrypto.Hash                                           // hash algorithm
		partitions    PartitionStore                                          // partition store
		ir            map[protocol.SystemIdentifier]*certificates.InputRecord // currently valid input records
		execStateId   []byte
		inProgress    map[protocol.SystemIdentifier]struct{}
		statePipeline map[uint64]*StateEntry
	}
)

func NewRoundPipeline(hash gocrypto.Hash, persistedState store.RootState, partitionStore PartitionStore) (*RoundPipeline, error) {
	//init IR map
	inputRecords := make(map[protocol.SystemIdentifier]*certificates.InputRecord)
	for id, cert := range persistedState.Certificates {
		inputRecords[id] = cert.InputRecord
	}
	// populate pending state with existing state
	return &RoundPipeline{
		HighQC:        nil,
		hashAlgorithm: hash,
		partitions:    partitionStore,
		ir:            inputRecords,
		execStateId:   persistedState.LatestRootHash,
		inProgress:    make(map[protocol.SystemIdentifier]struct{}),
		statePipeline: make(map[uint64]*StateEntry),
	}, nil
}

func (x *RoundPipeline) Reset(persistedState store.RootState) {
	// clear map, the states will never be committed anyway
	x.statePipeline = make(map[uint64]*StateEntry)
	x.inProgress = make(map[protocol.SystemIdentifier]struct{})
	x.ir = make(map[protocol.SystemIdentifier]*certificates.InputRecord)
	for id, cert := range persistedState.Certificates {
		x.ir[id] = cert.InputRecord
	}
	x.execStateId = persistedState.LatestRootHash
	return
}

func (x *RoundPipeline) IsChangeInPipeline(sysId protocol.SystemIdentifier) bool {
	_, f := x.inProgress[sysId]
	return f
}

func (x *RoundPipeline) GetExecStateId() []byte {
	return x.execStateId
}

func (x *RoundPipeline) removeCompleted(changes map[protocol.SystemIdentifier]*certificates.InputRecord) {
	for ch := range changes {
		delete(x.inProgress, ch)
	}
}

func (x *RoundPipeline) Update(qc *atomic_broadcast.QuorumCert) *store.RootState {
	if qc == nil {
		return nil
	}
	var commitState *store.RootState = nil
	// If the QC commits a state
	if len(qc.LedgerCommitInfo.CommitStateId) != 0 {
		state, found := x.statePipeline[qc.VoteInfo.ParentRound]
		if found {
			// Commit pending state if it has the same state hash
			if bytes.Equal(state.State.LatestRootHash, qc.LedgerCommitInfo.CommitStateId) {
				//todo: AB-548 create new UnicitySeal structure and complete certificates
				// return state for sending result to partition manager
				commitState = state.State
				x.removeCompleted(state.Changed)
				// remove completed round from pipeline
				delete(x.statePipeline, qc.VoteInfo.ParentRound)
			}
		}
	}
	// Add qc to pending state
	state, found := x.statePipeline[qc.VoteInfo.RootRound]
	if found {
		state.Qc = qc
	}
	x.HighQC = qc
	return commitState
}

func (x *RoundPipeline) Add(round uint64, changes map[protocol.SystemIdentifier]*certificates.InputRecord) ([]byte, error) {
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	for sysId := range changes {
		if _, f := x.inProgress[sysId]; f {
			return nil, fmt.Errorf("add state failed: partition %X has pending chnages in pipeline", sysId.Bytes())
		}
	}
	// apply changes
	logger.Debug("Round %v, changed input records are:", round)
	for id, ch := range changes {
		util.WriteDebugJsonLog(logger, fmt.Sprintf("Round %v partition %X IR:", round, id), ch)
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
	for sysId, ir := range changes {
		utCert, err := ut.GetCertificate(sysId.Bytes())
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
		certs[sysId] = certificate
	}
	x.execStateId = rootHash
	x.statePipeline[round] = &StateEntry{State: &store.RootState{
		LatestRound:    round,
		LatestRootHash: rootHash,
		Certificates:   certs,
	}, Changed: changes, Qc: nil}
	return rootHash, nil
}
