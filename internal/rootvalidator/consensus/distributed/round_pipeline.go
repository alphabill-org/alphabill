package distributed

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/consensus"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/store"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
)

const highQCKey = "highQC"

type (
	RoundState struct {
		State     *store.RootState
		InputData map[protocol.SystemIdentifier]*InputData
		Changed   map[protocol.SystemIdentifier]struct{}
		Qc        *atomic_broadcast.QuorumCert
	}

	InputData struct {
		ir   *certificates.InputRecord
		sdrh []byte
	}

	PipelineStore struct {
		highQC  *atomic_broadcast.QuorumCert // highest QC seen
		storage consensus.KeyValueStorage
	}

	RoundPipeline struct {
		hashAlgorithm gocrypto.Hash                            // hash algorithm
		currentState  map[protocol.SystemIdentifier]*InputData // currently valid input records
		execStateID   []byte                                   // current state hash based on input data
		statePipeline map[uint64]*RoundState
		storage       *PipelineStore
	}
	PipelineOption func(c *PipelineStore)
)

// copyMap - creates a copy of map. NB! it cannot handle map of maps
func copyMap[K, V comparable](m map[K]V) map[K]V {
	result := make(map[K]V)
	for k, v := range m {
		result[k] = v
	}
	return result
}

func WithPipelineStore(p consensus.KeyValueStorage) PipelineOption {
	return func(c *PipelineStore) {
		c.storage = p
	}
}

func newDataStoreFromOptions(opts []PipelineOption) *PipelineStore {
	ds := &PipelineStore{
		highQC:  nil,
		storage: nil,
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt(ds)
	}
	return ds
}

func (d *PipelineStore) setHighQC(hqc *atomic_broadcast.QuorumCert) error {
	if hqc == nil {
		return errors.New("high qc is is nil")
	}

	d.highQC = hqc
	if d.storage != nil {
		if err := d.storage.Write(highQCKey, hqc); err != nil {
			return err
		}
	}
	return nil
}

func (d *PipelineStore) getHighQC() *atomic_broadcast.QuorumCert {
	return d.highQC
}

func NewPipelineDataFromState(state *store.RootState) map[protocol.SystemIdentifier]*InputData {
	data := make(map[protocol.SystemIdentifier]*InputData)
	for id, cert := range state.Certificates {
		data[id] = &InputData{
			ir:   cert.InputRecord,
			sdrh: cert.UnicityTreeCertificate.SystemDescriptionHash,
		}
	}
	return data
}

func HighQcFromGenesisState(g *store.RootState) *atomic_broadcast.QuorumCert {
	for _, cert := range g.Certificates {
		return &atomic_broadcast.QuorumCert{
			VoteInfo:         cert.UnicitySeal.RootRoundInfo,
			LedgerCommitInfo: cert.UnicitySeal.CommitInfo,
			Signatures:       cert.UnicitySeal.Signatures,
		}
	}
	return nil
}

func NewRoundPipelineFromRootState(hash gocrypto.Hash, state *store.RootState, opts ...PipelineOption) (*RoundPipeline, error) {
	// initiate store
	ds := newDataStoreFromOptions(opts)
	if err := ds.setHighQC(HighQcFromGenesisState(state)); err != nil {
		return nil, err
	}
	// initiate from last persisted/committed state
	return &RoundPipeline{
		hashAlgorithm: hash,
		currentState:  NewPipelineDataFromState(state),
		execStateID:   state.RootHash, //persistedState.RootHash,
		statePipeline: make(map[uint64]*RoundState),
		storage:       ds,
	}, nil
}

func NewRoundPipeline(hash gocrypto.Hash, persistedState *store.RootState, p consensus.KeyValueStorage) (*RoundPipeline, error) {
	// initiate store
	if p == nil {
		return nil, fmt.Errorf("no persistent store")
	}
	var hQC *atomic_broadcast.QuorumCert = nil
	// initiate store (assume genesis, as it is initiated only once if persistent storage is provided)
	if err := p.Read(highQCKey, hQC); err != nil {
		return nil, fmt.Errorf("failed to store high qc, %w", err)
	}
	// initiate from last persisted/committed state
	return &RoundPipeline{
		hashAlgorithm: hash,
		currentState:  NewPipelineDataFromState(persistedState),
		execStateID:   nil, //persistedState.RootHash,
		statePipeline: make(map[uint64]*RoundState),
		storage:       &PipelineStore{storage: p, highQC: hQC},
	}, nil
}

func (x *RoundPipeline) Reset(persistedState *store.RootState) {
	// clear map, the states will never be committed anyway
	x.statePipeline = make(map[uint64]*RoundState)
	x.currentState = NewPipelineDataFromState(persistedState)
	x.execStateID = persistedState.RootHash
	return
}

func (x *RoundPipeline) IsChangeInPipeline(sysId protocol.SystemIdentifier) bool {
	for _, s := range x.statePipeline {
		if _, found := s.Changed[sysId]; found {
			return true
		}
	}
	return false
}

func (x *RoundPipeline) GetExecStateId() []byte {
	return x.execStateID
}

func (x *RoundPipeline) Update(qc *atomic_broadcast.QuorumCert) *store.RootState {
	if qc == nil {
		return nil
	}
	// remember the highest QC seen so far
	_ = x.storage.setHighQC(qc)
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
	if found && bytes.Equal(state.State.RootHash, qc.LedgerCommitInfo.RootHash) {
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
		delete(x.statePipeline, qc.VoteInfo.ParentRoundNumber)
		return commitState
	}
	return nil
}

// Add adds new round state to pipeline and returns the new state root hash a.k.a. execStateID
func (x *RoundPipeline) Add(round uint64, changes map[protocol.SystemIdentifier]*InputData) ([]byte, error) {
	// verify that state for the round does not exist yet
	if _, found := x.statePipeline[round]; found {
		return nil, fmt.Errorf("add state failed: state for round %v already in pipeline", round)
	}
	// verify that there are no pending changes in the pipeline for any of the updated partitions
	for sysID := range changes {
		if x.IsChangeInPipeline(sysID) {
			return nil, fmt.Errorf("add state failed: partition %X has pending changes in pipeline", sysID.Bytes())
		}
	}
	changed := map[protocol.SystemIdentifier]struct{}{}
	if len(changes) == 0 {
		logger.Debug("Round %v executing proposal, no changes to input records", round)
	} else {
		// apply changes
		logger.Debug("Round %v executing proposal, changed input records are:", round)
		for id, d := range changes {
			util.WriteDebugJsonLog(logger, fmt.Sprintf("partition %X IR:", id), d.ir)
			x.currentState[id] = d
			changed[id] = struct{}{}
		}
	}

	utData := make([]*unicitytree.Data, 0, len(x.currentState))
	for id, data := range x.currentState {
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            id.Bytes(),
			InputRecord:                 data.ir,
			SystemDescriptionRecordHash: data.sdrh,
		})
	}
	ut, err := unicitytree.New(x.hashAlgorithm.New(), utData)
	if err != nil {
		return nil, err
	}
	rootHash := ut.GetRootHash()
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for sysID, data := range changes {
		utCert, err := ut.GetCertificate(sysID.Bytes())
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			panic(err)
		}
		certificate := &certificates.UnicityCertificate{
			InputRecord: data.ir,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
		}
		certs[sysID] = certificate
	}
	x.execStateID = rootHash
	x.statePipeline[round] = &RoundState{
		InputData: copyMap(x.currentState),
		Changed:   changed,
		Qc:        nil,
		State: &store.RootState{
			Round:        round,
			RootHash:     rootHash,
			Certificates: certs,
		},
	}
	return rootHash, nil
}

func (x *RoundPipeline) GetHighQc() *atomic_broadcast.QuorumCert {
	return x.storage.getHighQC()
}
