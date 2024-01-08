package storage

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/types"
)

type (
	InputData struct {
		SysID types.SystemID     `json:"systemIdentifier"`
		IR    *types.InputRecord `json:"ir"`
		Sdrh  []byte             `json:"sdrh"` // System Description Record Hash
	}

	InputRecords []*InputData
	SysIDList    []types.SystemID

	ExecutedBlock struct {
		BlockData *drctypes.BlockData  `json:"block"`         // proposed block
		CurrentIR InputRecords         `json:"inputData"`     // all input records in this block
		Changed   SysIDList            `json:"changed"`       // changed partition system identifiers
		HashAlgo  gocrypto.Hash        `json:"hashAlgorithm"` // hash algorithm for the block
		RootHash  []byte               `json:"rootHash"`      // resulting root hash
		Qc        *drctypes.QuorumCert `json:"Qc"`            // block's quorum certificate (from next view)
		CommitQc  *drctypes.QuorumCert `json:"commitQc"`      // commit certificate
	}

	IRChangeReqVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *drctypes.IRChangeReq) (*InputData, error)
	}
)

func (data InputRecords) Update(newInputData *InputData) error {
	for i, d := range data {
		if d.SysID == newInputData.SysID {
			data[i] = newInputData
			return nil
		}
	}
	return fmt.Errorf("input record with system id %X was not found", newInputData.SysID)
}

func (data InputRecords) Find(sysID types.SystemID) *InputData {
	for _, d := range data {
		if d.SysID == sysID {
			return d
		}
	}
	return nil
}

func QcFromGenesisState(partitionRecords []*genesis.GenesisPartitionRecord) *drctypes.QuorumCert {
	for _, p := range partitionRecords {
		return &drctypes.QuorumCert{
			VoteInfo: &drctypes.RoundInfo{
				RoundNumber:       p.Certificate.UnicitySeal.RootChainRoundNumber,
				Epoch:             0,
				Timestamp:         p.Certificate.UnicitySeal.Timestamp,
				ParentRoundNumber: 0,
				CurrentRootHash:   p.Certificate.UnicitySeal.Hash,
			},
			LedgerCommitInfo: &types.UnicitySeal{
				PreviousHash:         p.Certificate.UnicitySeal.PreviousHash,
				RootChainRoundNumber: p.Certificate.UnicitySeal.RootChainRoundNumber,
				Hash:                 p.Certificate.UnicitySeal.Hash,
				Timestamp:            p.Certificate.UnicitySeal.Timestamp,
			},
			Signatures: p.Certificate.UnicitySeal.Signatures,
		}
	}
	return nil
}

func NewGenesisBlock(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord) *ExecutedBlock {
	data := make([]*InputData, len(pg))
	for i, partition := range pg {
		data[i] = &InputData{
			SysID: partition.SystemDescriptionRecord.SystemIdentifier,
			IR:    partition.Certificate.InputRecord,
			Sdrh:  partition.Certificate.UnicityTreeCertificate.SystemDescriptionHash,
		}
	}
	qc := QcFromGenesisState(pg)
	// If not initiated, save genesis file to store
	return &ExecutedBlock{
		BlockData: &drctypes.BlockData{
			Author:    "genesis",
			Round:     genesis.RootRound,
			Epoch:     0,
			Timestamp: genesis.Timestamp,
			Payload:   nil,
			Qc:        nil,
		},
		CurrentIR: data,
		Changed:   make([]types.SystemID, 0),
		HashAlgo:  hash,
		RootHash:  qc.LedgerCommitInfo.Hash,
		Qc:        qc, // qc to itself
		CommitQc:  qc, // use same qc to itself for genesis block
	}
}

func NewRootBlockFromRecovery(hash gocrypto.Hash, recoverBlock *abdrc.RecoveryBlock) (*ExecutedBlock, error) {
	var changes SysIDList
	if recoverBlock.Block.Payload != nil {
		changes = make([]types.SystemID, 0, len(recoverBlock.Block.Payload.Requests))
		// verify requests for IR change and proof of consensus
		for _, irChReq := range recoverBlock.Block.Payload.Requests {
			changes = append(changes, irChReq.SystemIdentifier)
		}
	}
	// recover input records
	irState := make(InputRecords, len(recoverBlock.Ir))
	for i, d := range recoverBlock.Ir {
		irState[i] = &InputData{
			SysID: d.SysID,
			IR:    d.Ir,
			Sdrh:  d.Sdrh,
		}
	}
	// calculate root hash
	utData := make([]*unicitytree.Data, 0, len(irState))
	for _, data := range irState {
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            data.SysID,
			InputRecord:                 data.IR,
			SystemDescriptionRecordHash: data.Sdrh,
		})
	}
	ut, err := unicitytree.New(hash.New(), utData)
	if err != nil {
		return nil, err
	}
	root := ut.GetRootHash()
	// check against qc and commit qc
	if recoverBlock.Qc != nil {
		if !bytes.Equal(root, recoverBlock.Qc.VoteInfo.CurrentRootHash) {
			return nil, fmt.Errorf("invalid recovery data, qc state hash does not match input records")
		}
	}
	if recoverBlock.CommitQc != nil {
		if !bytes.Equal(root, recoverBlock.CommitQc.LedgerCommitInfo.Hash) {
			return nil, fmt.Errorf("invalid recovery data, commit qc state hash does not match input records")
		}
	}
	return &ExecutedBlock{
		BlockData: recoverBlock.Block,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  bytes.Clone(ut.GetRootHash()),
		Qc:        recoverBlock.Qc,
		CommitQc:  recoverBlock.CommitQc,
	}, nil
}

func NewExecutedBlock(hash gocrypto.Hash, newBlock *drctypes.BlockData, parent *ExecutedBlock, verifier IRChangeReqVerifier) (*ExecutedBlock, error) {
	changed := make(InputRecords, 0, len(newBlock.Payload.Requests))
	changes := make([]types.SystemID, 0, len(newBlock.Payload.Requests))
	// verify requests for IR change and proof of consensus
	for _, irChReq := range newBlock.Payload.Requests {
		irData, err := verifier.VerifyIRChangeReq(newBlock.GetRound(), irChReq)
		if err != nil {
			return nil, fmt.Errorf("new block verification in round %v error, %w", newBlock.Round, err)
		}
		changed = append(changed, irData)
		changes = append(changes, irChReq.SystemIdentifier)
	}
	// copy parent input records
	irState := make(InputRecords, len(parent.CurrentIR))
	copy(irState, parent.CurrentIR)
	for _, d := range changed {
		if err := irState.Update(d); err != nil {
			return nil, fmt.Errorf("block execution failed, system id %X was not found in input records", d.SysID)
		}
	}
	// calculate root hash
	utData := make([]*unicitytree.Data, 0, len(irState))
	for _, data := range irState {
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            data.SysID,
			InputRecord:                 data.IR,
			SystemDescriptionRecordHash: data.Sdrh,
		})
	}
	ut, err := unicitytree.New(hash.New(), utData)
	if err != nil {
		return nil, err
	}
	return &ExecutedBlock{
		BlockData: newBlock,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  bytes.Clone(ut.GetRootHash()),
	}, nil
}

func (x *ExecutedBlock) generateUnicityTree() (*unicitytree.UnicityTree, error) {
	utData := make([]*unicitytree.Data, 0, len(x.CurrentIR))
	for _, data := range x.CurrentIR {
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &unicitytree.Data{
			SystemIdentifier:            data.SysID,
			InputRecord:                 data.IR,
			SystemDescriptionRecordHash: data.Sdrh,
		})
	}
	return unicitytree.New(x.HashAlgo.New(), utData)
}

func (x *ExecutedBlock) GenerateCertificates(commitQc *drctypes.QuorumCert) (map[types.SystemID]*types.UnicityCertificate, error) {
	ut, err := x.generateUnicityTree()
	if err != nil {
		return nil, fmt.Errorf("failed to generate unicity tree: %w", err)
	}
	rootHash := ut.GetRootHash()
	// sanity check, data must not have changed, hence the root hash must still be the same
	if !bytes.Equal(rootHash, x.RootHash) {
		return nil, fmt.Errorf("root hash does not match previously calculated root hash")
	}
	// sanity check, if root hashes do not match then fall back to recovery
	if !bytes.Equal(rootHash, commitQc.LedgerCommitInfo.Hash) {
		return nil, fmt.Errorf("commit of block round %v failed, root hash mismatch", commitQc.VoteInfo.ParentRoundNumber)
	}
	// Commit pending state if it has the same root hash as committed state
	// create UnicitySeal for pending certificates
	uSeal := &types.UnicitySeal{
		RootChainRoundNumber: commitQc.LedgerCommitInfo.RootChainRoundNumber,
		Hash:                 commitQc.LedgerCommitInfo.Hash,
		Timestamp:            commitQc.LedgerCommitInfo.Timestamp,
		PreviousHash:         commitQc.LedgerCommitInfo.PreviousHash,
		Signatures:           commitQc.Signatures,
	}
	ucs := map[types.SystemID]*types.UnicityCertificate{}
	// copy parent certificates and extract changed certificates from this round
	for _, sysID := range x.Changed {
		utCert, err := ut.GetCertificate(sysID)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			return nil, fmt.Errorf("failed to read certificate of %X: %w", sysID, err)
		}
		ir := x.CurrentIR.Find(sysID)
		if ir == nil {
			return nil, fmt.Errorf("input record for %X not found", sysID)
		}
		certificate := &types.UnicityCertificate{
			InputRecord: ir.IR,
			UnicityTreeCertificate: &types.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
			UnicitySeal: uSeal,
		}
		ucs[sysID] = certificate
	}
	x.CommitQc = commitQc
	return ucs, nil
}

func (x *ExecutedBlock) GetRound() uint64 {
	if x != nil {
		return x.BlockData.GetRound()
	}
	return 0
}

func (x *ExecutedBlock) GetParentRound() uint64 {
	if x != nil {
		return x.BlockData.GetParentRound()
	}
	return 0
}
