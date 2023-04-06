package storage

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rootchain/unicitytree"
	"github.com/alphabill-org/alphabill/internal/util"
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
		BlockData *atomic_broadcast.BlockData  `json:"block"`         // proposed block
		CurrentIR InputRecords                 `json:"inputData"`     // all input records in this block
		Changed   Changed                      `json:"changed"`       // changed partition system identifiers
		HashAlgo  gocrypto.Hash                `json:"hashAlgorithm"` // hash algorithm for the block
		RootHash  []byte                       `json:"rootHash"`      // resulting root hash
		Qc        *atomic_broadcast.QuorumCert `json:"Qc"`            // block's quorum certificate (from next view)
		CommitQc  *atomic_broadcast.QuorumCert `json:"commitQc"`      // commit certificate
	}

	IRChangeReqVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *atomic_broadcast.IRChangeReqMsg) (*InputData, error)
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

func (data *Changed) Find(sysID []byte) bool {
	if data == nil {
		return false
	}
	for _, d := range *data {
		if bytes.Equal(d, sysID) {
			return true
		}
	}
	return false
}

func QcFromGenesisState(partitionRecords []*genesis.GenesisPartitionRecord) *atomic_broadcast.QuorumCert {
	for _, p := range partitionRecords {
		return &atomic_broadcast.QuorumCert{
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
		BlockData: &atomic_broadcast.BlockData{
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

func NewExecutedBlockFromRecovery(hash gocrypto.Hash, recoverBlock *atomic_broadcast.RecoveryBlock, verifier IRChangeReqVerifier) (*ExecutedBlock, error) {
	changed := make(InputRecords, 0, len(recoverBlock.Block.Payload.Requests))
	changes := make([][]byte, 0, len(recoverBlock.Block.Payload.Requests))
	// verify requests for IR change and proof of consensus
	for _, irChReq := range recoverBlock.Block.Payload.Requests {
		irData, err := verifier.VerifyIRChangeReq(recoverBlock.Block.Round, irChReq)
		if err != nil {
			return nil, fmt.Errorf("new block verification in round %v error, %w", recoverBlock.Block.Round, err)
		}
		changed = append(changed, irData)
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
		if bytes.Equal(root, recoverBlock.Qc.VoteInfo.CurrentRootHash) == false {
			return nil, fmt.Errorf("invalid recovery data, qc state hash does not match input records")
		}
	}
	if recoverBlock.CommitQc != nil {
		if bytes.Equal(root, recoverBlock.CommitQc.LedgerCommitInfo.RootHash) == false {
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

func NewExecutedBlock(hash gocrypto.Hash, newBlock *atomic_broadcast.BlockData, parent *ExecutedBlock, verifier IRChangeReqVerifier) (*ExecutedBlock, error) {
	changed := make(InputRecords, 0, len(newBlock.Payload.Requests))
	changes := make([][]byte, 0, len(newBlock.Payload.Requests))
	// verify requests for IR change and proof of consensus
	for _, irChReq := range newBlock.Payload.Requests {
		irData, err := verifier.VerifyIRChangeReq(newBlock.Round, irChReq)
		if err != nil {
			return nil, fmt.Errorf("new block verification in round %v error, %w", newBlock.Round, err)
		}
		changed = append(changed, irData)
		changes = append(changes, irChReq.SystemIdentifier)
	}
	// copy parent input records
	irState := make(InputRecords, len(parent.CurrentIR))
	copy(irState, parent.CurrentIR)
	if len(changed) == 0 {
		logger.Trace("Round %v executing proposal, no changes to input records", newBlock.Round)
	} else {
		// apply changes
		logger.Trace("Round %v executing proposal, changed input records are:", newBlock.Round)
		for _, d := range changed {
			util.WriteTraceJsonLog(logger, fmt.Sprintf("partition %X IR:", d.SysID), d.IR)
			if err := irState.Update(d); err != nil {
				return nil, fmt.Errorf("block execution failed, system id %X was not found in input records", d.SysID)
			}
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

func (x *ExecutedBlock) GenerateCertificates(commitQc *atomic_broadcast.QuorumCert) (map[protocol.SystemIdentifier]*certificates.UnicityCertificate, error) {
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
