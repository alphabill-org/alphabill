package storage

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
)

type (
	InputData struct {
		SysID []byte                    `json:"systemIdentifier"`
		IR    *certificates.InputRecord `json:"ir"`
		Sdrh  []byte                    `json:"sdrh"`
	}

	InputRecords []*InputData
	Changed      [][]byte

	ExecutedBlock struct {
		BlockData *ab_consensus.BlockData  `json:"block"`         // proposed block
		CurrentIR InputRecords             `json:"inputData"`     // all input records in this block
		Changed   Changed                  `json:"changed"`       // changed partition system identifiers
		HashAlgo  gocrypto.Hash            `json:"hashAlgorithm"` // hash algorithm for the block
		RootHash  []byte                   `json:"rootHash"`      // resulting root hash
		Qc        *ab_consensus.QuorumCert `json:"Qc"`            // block's quorum certificate (from next view)
		CommitQc  *ab_consensus.QuorumCert `json:"commitQc"`      // commit certificate
	}

	IRChangeReqVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *ab_consensus.IRChangeReqMsg) (*InputData, error)
	}
)

func (data *InputRecords) Update(newInputData *InputData) error {
	if data == nil {
		return fmt.Errorf("input records is nil")
	}
	for i, d := range *data {
		if bytes.Equal(d.SysID, newInputData.SysID) {
			(*data)[i] = newInputData
			return nil
		}
	}
	return fmt.Errorf("input record with system id %X was not found", newInputData.SysID)
}

func (data *InputRecords) Find(sysID []byte) *InputData {
	if data == nil {
		return nil
	}
	for _, d := range *data {
		if bytes.Equal(d.SysID, sysID) {
			return d
		}
	}
	return nil
}

func (data Changed) Contains(sysID []byte) bool {
	for _, d := range data {
		if bytes.Equal(d, sysID) {
			return true
		}
	}
	return false
}

func QcFromGenesisState(partitionRecords []*genesis.GenesisPartitionRecord) *ab_consensus.QuorumCert {
	for _, p := range partitionRecords {
		return &ab_consensus.QuorumCert{
			VoteInfo:         p.Certificate.UnicitySeal.RootRoundInfo,
			LedgerCommitInfo: p.Certificate.UnicitySeal.CommitInfo,
			Signatures:       p.Certificate.UnicitySeal.Signatures,
		}
	}
	return nil
}

func NewExecutedBlockFromGenesis(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord) *ExecutedBlock {
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
		BlockData: &ab_consensus.BlockData{
			Author:    "genesis",
			Round:     genesis.RootRound,
			Epoch:     0,
			Timestamp: genesis.Timestamp,
			Payload:   nil,
			Qc:        nil,
		},
		CurrentIR: data,
		Changed:   make([][]byte, 0),
		HashAlgo:  hash,
		RootHash:  qc.LedgerCommitInfo.RootHash,
		Qc:        qc, // qc to itself
		CommitQc:  qc, // use same qc to itself for genesis block
	}
}

func NewExecutedBlockFromRecovery(hash gocrypto.Hash, recoverBlock *ab_consensus.RecoveryBlock, verifier IRChangeReqVerifier) (*ExecutedBlock, error) {
	changes := make([][]byte, 0, len(recoverBlock.Block.Payload.Requests))
	// verify requests for IR change and proof of consensus
	for _, irChReq := range recoverBlock.Block.Payload.Requests {
		_, err := verifier.VerifyIRChangeReq(recoverBlock.Block.GetRound(), irChReq)
		if err != nil {
			return nil, fmt.Errorf("new block verification in round %v error, %w", recoverBlock.Block.Round, err)
		}
		changes = append(changes, irChReq.SystemIdentifier)
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
	root := ut.GetRootHash()
	// check against qc and commit qc
	if recoverBlock.Qc != nil {
		if !bytes.Equal(root, recoverBlock.Qc.VoteInfo.CurrentRootHash) {
			return nil, fmt.Errorf("invalid recovery data, qc state hash does not match input records")
		}
	}
	if recoverBlock.CommitQc != nil {
		if !bytes.Equal(root, recoverBlock.CommitQc.LedgerCommitInfo.RootHash) {
			return nil, fmt.Errorf("invalid recovery data, commit qc state hash does not match input records")
		}
	}
	return &ExecutedBlock{
		BlockData: recoverBlock.Block,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  ut.GetRootHash(),
		Qc:        recoverBlock.Qc,
		CommitQc:  recoverBlock.CommitQc,
	}, nil
}

func NewExecutedBlock(hash gocrypto.Hash, newBlock *ab_consensus.BlockData, parent *ExecutedBlock, verifier IRChangeReqVerifier) (*ExecutedBlock, error) {
	changed := make(InputRecords, 0, len(newBlock.Payload.Requests))
	changes := make([][]byte, 0, len(newBlock.Payload.Requests))
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
		RootHash:  ut.GetRootHash(),
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

func (x *ExecutedBlock) GenerateCertificates(commitQc *ab_consensus.QuorumCert) (map[protocol.SystemIdentifier]*certificates.UnicityCertificate, error) {
	ut, err := x.generateUnicityTree()
	if err != nil {
		return nil, fmt.Errorf("unexpected error, failed to generate unicity tree, %w", err)
	}
	rootHash := ut.GetRootHash()
	// sanity check, data must not have changed, hence the root hash must still be the same
	if !bytes.Equal(rootHash, x.RootHash) {
		return nil, fmt.Errorf("unexpected error, root hash does not match previously calculated root hash")
	}
	// sanity check, if root hashes do not match then fall back to recovery
	if !bytes.Equal(rootHash, commitQc.LedgerCommitInfo.RootHash) {
		return nil, fmt.Errorf("commit of block round %v failed, root hash mismatch", commitQc.VoteInfo.ParentRoundNumber)
	}
	// Commit pending state if it has the same root hash as committed state
	// create UnicitySeal for pending certificates
	uSeal := &certificates.UnicitySeal{
		RootRoundInfo: commitQc.VoteInfo,
		CommitInfo:    commitQc.LedgerCommitInfo,
		Signatures:    commitQc.Signatures,
	}
	ucs := map[protocol.SystemIdentifier]*certificates.UnicityCertificate{}
	// copy parent certificates and extract changed certificates from this round
	for _, sysID := range x.Changed {
		var utCert *certificates.UnicityTreeCertificate
		utCert, err = ut.GetCertificate(sysID)
		if err != nil {
			// this should never happen. if it does then exit with panic because we cannot generate
			// unicity tree certificates.
			return nil, fmt.Errorf("unexpected error, failed to generate unicity certificates, %w", err)
		}
		ir := x.CurrentIR.Find(sysID)
		if ir == nil {
			return nil, fmt.Errorf("unexpected error, failed to generate unicity certificates, input record for %X not found", sysID)
		}
		certificate := &certificates.UnicityCertificate{
			InputRecord: ir.IR,
			UnicityTreeCertificate: &certificates.UnicityTreeCertificate{
				SystemIdentifier:      utCert.SystemIdentifier,
				SiblingHashes:         utCert.SiblingHashes,
				SystemDescriptionHash: utCert.SystemDescriptionHash,
			},
			UnicitySeal: uSeal,
		}
		ucs[protocol.SystemIdentifier(sysID)] = certificate
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