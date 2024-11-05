package storage

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	rcgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
)

type (
	InputData struct {
		_         struct{} `cbor:",toarray"`
		Partition types.PartitionID
		Shard     types.ShardID
		IR        *types.InputRecord
		Technical certification.TechnicalRecord
		PDRHash   hex.Bytes // Partition Description Record Hash
	}

	InputRecords []*InputData

	ExecutedBlock struct {
		_         struct{}                    `cbor:",toarray"`
		BlockData *rctypes.BlockData          // proposed block
		CurrentIR InputRecords                // all input records in this block
		Changed   map[partitionShard]struct{} // changed shard identifiers
		HashAlgo  crypto.Hash                 // hash algorithm for the block
		RootHash  hex.Bytes                   // resulting root hash
		Qc        *rctypes.QuorumCert         // block's quorum certificate (from next view)
		CommitQc  *rctypes.QuorumCert         // block's commit certificate
	}

	IRChangeReqVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *rctypes.IRChangeReq) (*InputData, error)
	}
)

func (data InputRecords) Update(newInputData *InputData) error {
	for i, d := range data {
		if d.Partition == newInputData.Partition {
			data[i] = newInputData
			return nil
		}
	}
	return fmt.Errorf("input record for partition %s was not found", newInputData.Partition)
}

func (data InputRecords) Find(sysID types.PartitionID) *InputData {
	for _, d := range data {
		if d.Partition == sysID {
			return d
		}
	}
	return nil
}

/*
unicityTree builds the unicity tree based on the InputData slice.
*/
func (data InputRecords) unicityTree(algo crypto.Hash) (*types.UnicityTree, error) {
	// TODO: supports just single shard partitions, ie each element in the data slice is the sole shard of the partition!
	utData := make([]*types.UnicityTreeData, 0, len(data))
	for _, d := range data {
		trHash, err := d.Technical.Hash()
		if err != nil {
			return nil, fmt.Errorf("calculating TR hash: %w", err)
		}
		sTree, err := types.CreateShardTree(types.ShardingScheme{}, []types.ShardTreeInput{{Shard: d.Shard, IR: d.IR, TRHash: trHash}}, algo)
		if err != nil {
			return nil, fmt.Errorf("creating shard tree: %w", err)
		}
		utData = append(utData, &types.UnicityTreeData{
			Partition:     d.Partition,
			ShardTreeRoot: sTree.RootHash(),
			PDRHash:       d.PDRHash,
		})
	}
	return types.NewUnicityTree(algo, utData)
}

/*
certificationResponses builds the unicity tree and certification responses based on the InputData slice.
CertificationResponse will be generated only for shards listed in the "changed" argument. The UnicityCertificates
in the response are not complete, they miss the UnicityTreeCertificate and UnicitySeal.
*/
func (data InputRecords) certificationResponses(changed map[partitionShard]struct{}, algo crypto.Hash) ([]*certification.CertificationResponse, *types.UnicityTree, error) {
	crs := []*certification.CertificationResponse{}
	utData := make([]*types.UnicityTreeData, 0, len(data))
	for _, d := range data {
		trHash, err := d.Technical.Hash()
		if err != nil {
			return nil, nil, fmt.Errorf("calculating TR hash: %w", err)
		}
		sTree, err := types.CreateShardTree(types.ShardingScheme{}, []types.ShardTreeInput{{Shard: d.Shard, IR: d.IR, TRHash: trHash}}, algo)
		if err != nil {
			return nil, nil, fmt.Errorf("creating shard tree: %w", err)
		}
		utData = append(utData, &types.UnicityTreeData{
			Partition:     d.Partition,
			ShardTreeRoot: sTree.RootHash(),
			PDRHash:       d.PDRHash,
		})

		if _, ok := changed[partitionShard{partition: d.Partition, shard: d.Shard.Key()}]; ok {
			stCert, err := sTree.Certificate(d.Shard)
			if err != nil {
				return nil, nil, fmt.Errorf("creating shard tree certificate: %w", err)
			}
			crs = append(crs, &certification.CertificationResponse{
				Partition: d.Partition,
				Shard:     d.Shard,
				Technical: d.Technical,
				UC: types.UnicityCertificate{
					Version:              1,
					InputRecord:          d.IR,
					TRHash:               trHash,
					ShardTreeCertificate: stCert,
				},
			})
		}
	}
	ut, err := types.NewUnicityTree(algo, utData)
	return crs, ut, err
}

func qcFromGenesisState(partitionRecords []*genesis.GenesisPartitionRecord) *rctypes.QuorumCert {
	for _, p := range partitionRecords {
		return &rctypes.QuorumCert{
			VoteInfo: &rctypes.RoundInfo{
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

func NewGenesisBlock(hash crypto.Hash, pg []*genesis.GenesisPartitionRecord) (*ExecutedBlock, error) {
	var err error
	data := make([]*InputData, len(pg))
	for i, partition := range pg {
		data[i] = &InputData{
			Partition: partition.PartitionDescription.PartitionIdentifier,
			Shard:     types.ShardID{},
			IR:        partition.Certificate.InputRecord,
			PDRHash:   partition.Certificate.UnicityTreeCertificate.PDRHash,
		}
		nodeIDs := util.TransformSlice(partition.Nodes, func(pn *genesis.PartitionNode) string { return pn.NodeIdentifier })
		if data[i].Technical, err = rcgenesis.TechnicalRecord(partition.Certificate.InputRecord, nodeIDs); err != nil {
			return nil, fmt.Errorf("creating TechnicalRecord: %w", err)
		}
	}
	qc := qcFromGenesisState(pg)
	// If not initiated, save genesis file to store
	return &ExecutedBlock{
		BlockData: &rctypes.BlockData{
			Author:    "genesis",
			Round:     genesis.RootRound,
			Epoch:     0,
			Timestamp: genesis.Timestamp,
			Payload:   nil,
			Qc:        qc, // qc to itself
		},
		CurrentIR: data,
		Changed:   make(map[partitionShard]struct{}),
		HashAlgo:  hash,
		RootHash:  qc.LedgerCommitInfo.Hash,
		Qc:        qc, // qc to itself
		CommitQc:  qc, // use same qc to itself for genesis block
	}, nil
}

func NewRootBlock(hash crypto.Hash, block *abdrc.CommittedBlock) (*ExecutedBlock, error) {
	var changes map[partitionShard]struct{}
	if block.Block.Payload != nil {
		changes = make(map[partitionShard]struct{})
		// verify requests for IR change and proof of consensus
		for _, irChReq := range block.Block.Payload.Requests {
			changes[partitionShard{partition: irChReq.Partition, shard: irChReq.Shard.Key()}] = struct{}{}
		}
	}
	// recover input records
	irState := make(InputRecords, len(block.Ir))
	for i, d := range block.Ir {
		irState[i] = &InputData{
			Partition: d.Partition,
			Shard:     d.Shard,
			IR:        d.Ir,
			Technical: d.Technical,
			PDRHash:   d.PDRHash,
		}
	}
	ut, err := irState.unicityTree(hash)
	if err != nil {
		return nil, err
	}
	return &ExecutedBlock{
		BlockData: block.Block,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  ut.RootHash(),
		Qc:        block.Qc,       // qc to itself
		CommitQc:  block.CommitQc, // use same qc to itself for genesis block
	}, nil
}

type getTRFunc func(types.PartitionID, types.ShardID, *certification.BlockCertificationRequest) (certification.TechnicalRecord, error)

func NewExecutedBlock(hash crypto.Hash, newBlock *rctypes.BlockData, parent *ExecutedBlock, verifier IRChangeReqVerifier, getTR getTRFunc) (*ExecutedBlock, error) {
	changed := make(InputRecords, 0, len(newBlock.Payload.Requests))
	changes := make(map[partitionShard]struct{})
	// verify requests for IR change and proof of consensus
	for _, irChReq := range newBlock.Payload.Requests {
		irData, err := verifier.VerifyIRChangeReq(newBlock.GetRound(), irChReq)
		if err != nil {
			return nil, fmt.Errorf("new block verification in round %v error, %w", newBlock.Round, err)
		}
		// timeout IR change request do not have BCR
		var req *certification.BlockCertificationRequest
		if len(irChReq.Requests) > 0 {
			req = irChReq.Requests[0]
		}
		tr, err := getTR(irChReq.Partition, irChReq.Shard, req)
		if err != nil {
			return nil, fmt.Errorf("get TechnicalRecord: %w", err)
		}
		irData.Technical = tr
		changed = append(changed, irData)
		changes[partitionShard{partition: irChReq.Partition, shard: irChReq.Shard.Key()}] = struct{}{}
	}
	// copy parent input records
	irState := make(InputRecords, len(parent.CurrentIR))
	copy(irState, parent.CurrentIR)
	for _, d := range changed {
		if err := irState.Update(d); err != nil {
			return nil, fmt.Errorf("block execution failed, no input record for partition %s", d.Partition)
		}
	}
	ut, err := irState.unicityTree(hash)
	if err != nil {
		return nil, fmt.Errorf("creating UnicityTree: %w", err)
	}
	return &ExecutedBlock{
		BlockData: newBlock,
		CurrentIR: irState,
		Changed:   changes,
		HashAlgo:  hash,
		RootHash:  ut.RootHash(),
	}, nil
}

func (x *ExecutedBlock) GenerateCertificates(commitQc *rctypes.QuorumCert) ([]*certification.CertificationResponse, error) {
	crs, ut, err := x.CurrentIR.certificationResponses(x.Changed, x.HashAlgo)
	if err != nil {
		return nil, fmt.Errorf("failed to generate unicity tree: %w", err)
	}
	rootHash := ut.RootHash()
	// sanity check, data must not have changed, hence the root hash must still be the same
	if !bytes.Equal(rootHash, x.RootHash) {
		return nil, fmt.Errorf("root hash does not match previously calculated root hash")
	}
	// sanity check, if root hashes do not match then fall back to recovery
	if !bytes.Equal(rootHash, commitQc.LedgerCommitInfo.Hash) {
		return nil, fmt.Errorf("commit of block round %v failed, root hash mismatch", commitQc.VoteInfo.ParentRoundNumber)
	}
	// create UnicitySeal for pending certificates
	uSeal := &types.UnicitySeal{
		Version:              1,
		RootChainRoundNumber: commitQc.LedgerCommitInfo.RootChainRoundNumber,
		Hash:                 commitQc.LedgerCommitInfo.Hash,
		Timestamp:            commitQc.LedgerCommitInfo.Timestamp,
		PreviousHash:         commitQc.LedgerCommitInfo.PreviousHash,
		Signatures:           commitQc.Signatures,
	}
	ucs := []*certification.CertificationResponse{}
	for _, cr := range crs {
		if cr.UC.UnicityTreeCertificate, err = ut.Certificate(cr.Partition); err != nil {
			return nil, fmt.Errorf("create unicity tree certificate for partition %s - %s: %w", cr.Partition, cr.Shard, err)
		}
		cr.UC.UnicitySeal = uSeal
		ucs = append(ucs, cr)
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
