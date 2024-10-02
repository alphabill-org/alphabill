package storage

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/rootchain/unicitytree"
)

type (
	InputData struct {
		_     struct{} `cbor:",toarray"`
		SysID types.SystemID
		IR    *types.InputRecord
		Sdrh  []byte // System Description Record Hash
	}

	InputRecords []*InputData
	SysIDList    []types.SystemID

	ExecutedBlock struct {
		_         struct{}             `cbor:",toarray"`
		BlockData *drctypes.BlockData  // proposed block
		CurrentIR InputRecords         // all input records in this block
		Changed   SysIDList            // changed partition system identifiers
		HashAlgo  gocrypto.Hash        // hash algorithm for the block
		RootHash  []byte               // resulting root hash
		Qc        *drctypes.QuorumCert // block's quorum certificate (from next view)
		CommitQc  *drctypes.QuorumCert // block's commit certificate
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
				Version:              1,
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
			SysID: partition.PartitionDescription.SystemIdentifier,
			IR:    partition.Certificate.InputRecord,
			Sdrh:  partition.Certificate.UnicityTreeCertificate.PartitionDescriptionHash,
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
			Qc:        qc, // qc to itself
		},
		CurrentIR: data,
		Changed:   make([]types.SystemID, 0),
		HashAlgo:  hash,
		RootHash:  qc.LedgerCommitInfo.Hash,
		Qc:        qc, // qc to itself
		CommitQc:  qc, // use same qc to itself for genesis block
	}
}

func NewRootBlock(hash gocrypto.Hash, block *abdrc.CommittedBlock) (*ExecutedBlock, error) {
	var changes SysIDList
	if block.Block.Payload != nil {
		changes = make([]types.SystemID, 0, len(block.Block.Payload.Requests))
		// verify requests for IR change and proof of consensus
		for _, irChReq := range block.Block.Payload.Requests {
			changes = append(changes, irChReq.SystemIdentifier)
		}
	}
	// recover input records
	irState := make(InputRecords, len(block.Ir))
	for i, d := range block.Ir {
		irState[i] = &InputData{
			SysID: d.SysID,
			IR:    d.Ir,
			Sdrh:  d.Sdrh,
		}
	}
	// calculate root hash
	utData := make([]*types.UnicityTreeData, 0, len(irState))
	for _, data := range irState {
		utData = append(utData, &types.UnicityTreeData{
			SystemIdentifier:         data.SysID,
			InputRecord:              data.IR,
			PartitionDescriptionHash: data.Sdrh,
		})
	}
	ut, err := unicitytree.New(hash, utData)
	if err != nil {
		return nil, err
	}
	return &ExecutedBlock{
		BlockData: block.Block,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  bytes.Clone(ut.GetRootHash()),
		Qc:        block.Qc,       // qc to itself
		CommitQc:  block.CommitQc, // use same qc to itself for genesis block
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
	utData := make([]*types.UnicityTreeData, 0, len(irState))
	for _, data := range irState {
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &types.UnicityTreeData{
			SystemIdentifier:         data.SysID,
			InputRecord:              data.IR,
			PartitionDescriptionHash: data.Sdrh,
		})
	}
	ut, err := unicitytree.New(hash, utData)
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
	utData := make([]*types.UnicityTreeData, 0, len(x.CurrentIR))
	for _, data := range x.CurrentIR {
		// if it is valid it must have at least one validator with a valid certification request
		// if there is more, all input records are matching
		utData = append(utData, &types.UnicityTreeData{
			SystemIdentifier:         data.SysID,
			InputRecord:              data.IR,
			PartitionDescriptionHash: data.Sdrh,
		})
	}
	return unicitytree.New(x.HashAlgo, utData)
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
		Version:              1,
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
				SystemIdentifier:         utCert.SystemIdentifier,
				HashSteps:                utCert.HashSteps,
				PartitionDescriptionHash: utCert.PartitionDescriptionHash,
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
