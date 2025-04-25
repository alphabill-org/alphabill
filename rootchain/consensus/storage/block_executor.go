package storage

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	ShardSet map[types.PartitionShardID]struct{}

	ExecutedBlock struct {
		_         struct{}            `cbor:",toarray"`
		BlockData *rctypes.BlockData  // proposed block
		HashAlgo  crypto.Hash         // hash algorithm for the block
		RootHash  hex.Bytes           // resulting root hash
		Qc        *rctypes.QuorumCert // block's quorum certificate (from next view)
		CommitQc  *rctypes.QuorumCert // block's commit certificate
		ShardInfo ShardStates

		// cache schemes of the block. Public as tests outside the package need to
		// be able to access it (refactor!)
		Schemes map[types.PartitionID]types.ShardingScheme `cbor:"-"`
	}

	IRChangeReqVerifier interface {
		VerifyIRChangeReq(round uint64, irChReq *rctypes.IRChangeReq) (*types.InputRecord, error)
	}
)

func NewRootBlock(block *abdrc.CommittedBlock, hash crypto.Hash, orchestration Orchestration) (*ExecutedBlock, error) {
	shardConfs, err := orchestration.ShardConfigs(block.GetRound())
	if err != nil {
		return nil, fmt.Errorf("loading shard configurations for round %d: %w", block.GetRound(), err)
	}
	if len(block.ShardInfo) != len(shardConfs) {
		return nil, fmt.Errorf("round %d has %d shards, block has data for %d shards", block.GetRound(), len(shardConfs), len(block.ShardInfo))
	}

	shardInfo := ShardStates{
		States:  make(map[types.PartitionShardID]*ShardInfo, len(shardConfs)),
		Changed: ShardSet{},
	}
	for _, d := range block.ShardInfo {
		shardKey := types.PartitionShardID{PartitionID: d.Partition, ShardID: d.Shard.Key()}
		shardConf, ok := shardConfs[shardKey]
		if !ok {
			return nil, fmt.Errorf("no shard conf for %s - %s", d.Partition, d.Shard)
		}
		shardConfHash, err := shardConf.Hash(crypto.SHA256)
		if err != nil {
			return nil, fmt.Errorf("calculating PDR hash: %w", err)
		}
		if !bytes.Equal(d.ShardConfHash, shardConfHash) {
			return nil, fmt.Errorf("calculated shard conf hash doesn't match the value in block data for %s - %s", d.Partition, d.Shard)
		}

		si := &ShardInfo{
			PartitionID:   d.Partition,
			ShardID:       d.Shard,
			T2Timeout:     d.T2Timeout,
			ShardConfHash: d.ShardConfHash,
			RootHash:      d.RootHash,
			PrevEpochStat: d.PrevEpochStat,
			Stat:          d.Stat,
			PrevEpochFees: d.PrevEpochFees,
			Fees:          d.Fees,
			IR:            d.IR,
			TR:            d.IRTR,
		}
		if d.UC != nil {
			si.LastCR = &certification.CertificationResponse{
				Partition: d.Partition,
				Shard:     d.Shard,
				Technical: *d.TR,
				UC:        *d.UC,
			}
		}
		if err := si.resetTrustBase(shardConf); err != nil {
			return nil, fmt.Errorf("initializing shard trustbase: %w", err)
		}
		shardInfo.States[shardKey] = si
	}

	/* no need to mark these shards as "changed", this is committed block with CertRsp already generated?
	if block.Block.Payload != nil {
		// verify requests for IR change and proof of consensus
		for _, irChReq := range block.Block.Payload.Requests {
			shardInfo.Changed[types.PartitionShardID{PartitionID: irChReq.Partition, ShardID: irChReq.Shard.Key()}] = struct{}{}
		}
	}*/

	schemes := shardingSchemes(shardConfs)
	ut, _, err := shardInfo.UnicityTree(schemes, hash)
	if err != nil {
		return nil, err
	}
	return &ExecutedBlock{
		BlockData: block.Block,
		HashAlgo:  hash,
		RootHash:  ut.RootHash(),
		Qc:        block.Qc,
		CommitQc:  block.CommitQc,
		ShardInfo: shardInfo,
		Schemes:   schemes,
	}, nil
}

func (x *ExecutedBlock) Extend(newBlock *rctypes.BlockData, verifier IRChangeReqVerifier, orchestration Orchestration, hash crypto.Hash) (*ExecutedBlock, error) {
	// clone parent state
	shardConfs, err := orchestration.ShardConfigs(newBlock.Round)
	if err != nil {
		return nil, fmt.Errorf("loading shard configurations for round %d: %w", newBlock.Round, err)
	}

	shardInfo, err := x.ShardInfo.nextBlock(shardConfs, hash)
	if err != nil {
		return nil, fmt.Errorf("creating shard info for the block: %w", err)
	}

	for _, irChReq := range newBlock.Payload.Requests {
		shardKey := types.PartitionShardID{PartitionID: irChReq.Partition, ShardID: irChReq.Shard.Key()}
		si, ok := shardInfo.States[shardKey]
		if !ok {
			return nil, fmt.Errorf("block contains data for shard %s - %s which is not active in round %d", irChReq.Partition, irChReq.Shard, newBlock.Round)
		}

		if si.IR, err = verifier.VerifyIRChangeReq(newBlock.Round, irChReq); err != nil {
			return nil, fmt.Errorf("verifying change request: %w", err)
		}

		// timeout IR change request do not have BCR
		var req *certification.BlockCertificationRequest
		if len(irChReq.Requests) > 0 {
			req = irChReq.Requests[0]
		}
		if err = si.nextRound(req, shardConfs[shardKey], hash); err != nil {
			return nil, fmt.Errorf("updating shard info for the next round: %w", err)
		}

		shardInfo.Changed[shardKey] = struct{}{}
	}

	schemes := shardingSchemes(shardConfs)
	ut, _, err := shardInfo.UnicityTree(schemes, hash)
	if err != nil {
		return nil, fmt.Errorf("creating UnicityTree: %w", err)
	}
	return &ExecutedBlock{
		BlockData: newBlock,
		HashAlgo:  hash,
		RootHash:  ut.RootHash(),
		ShardInfo: shardInfo,
		Schemes:   schemes,
	}, nil
}

func (x *ExecutedBlock) GenerateCertificates(commitQc *rctypes.QuorumCert) ([]*certification.CertificationResponse, error) {
	crs, rootHash, err := x.ShardInfo.certificationResponses(x.Schemes, x.HashAlgo)
	if err != nil {
		return nil, fmt.Errorf("failed to generate root hash: %w", err)
	}
	// sanity check, data must not have changed, hence the root hash must still be the same
	if !bytes.Equal(rootHash, x.RootHash) {
		return nil, fmt.Errorf("root hash does not match previously calculated root hash")
	}
	// sanity check, if root hashes do not match then fall back to recovery
	if !bytes.Equal(rootHash, commitQc.LedgerCommitInfo.Hash) {
		return nil, fmt.Errorf("root hash does not match hash in commit QC")
	}
	if len(crs) == 0 {
		return nil, nil
	}

	// create UnicitySeal for pending certificates
	uSeal := &types.UnicitySeal{
		Version:              1,
		NetworkID:            commitQc.LedgerCommitInfo.NetworkID,
		RootChainRoundNumber: commitQc.LedgerCommitInfo.RootChainRoundNumber,
		Epoch:                commitQc.LedgerCommitInfo.Epoch,
		Hash:                 commitQc.LedgerCommitInfo.Hash,
		Timestamp:            commitQc.LedgerCommitInfo.Timestamp,
		PreviousHash:         commitQc.LedgerCommitInfo.PreviousHash,
		Signatures:           commitQc.Signatures,
	}
	for _, cr := range crs {
		cr.UC.UnicitySeal = uSeal
		x.ShardInfo.States[types.PartitionShardID{PartitionID: cr.Partition, ShardID: cr.Shard.Key()}].LastCR = cr
	}
	return crs, nil
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

/*
shardSetItem is helper type for serializing ShardSet - map with complex key
is not handled properly by the CBOR library so we serialize it as array.
*/
type shardSetItem struct {
	_         struct{} `cbor:",toarray"`
	Partition types.PartitionID
	Shard     []byte
}

func (ss ShardSet) MarshalCBOR() ([]byte, error) {
	// map with complex key is not handled properly by the CBOR library so we serialize it as array
	d := make([]shardSetItem, len(ss))
	idx := 0
	for k := range ss {
		d[idx].Partition = k.PartitionID
		d[idx].Shard = []byte(k.ShardID)
		idx++
	}
	buf := bytes.Buffer{}
	if err := types.Cbor.Encode(&buf, d); err != nil {
		return nil, fmt.Errorf("encoding shard set data: %w", err)
	}
	return buf.Bytes(), nil
}

func (ss *ShardSet) UnmarshalCBOR(data []byte) error {
	var d []shardSetItem
	if err := types.Cbor.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("decoding shard set data: %w", err)
	}
	ssn := make(ShardSet, len(d))
	for _, itm := range d {
		ssn[types.PartitionShardID{PartitionID: itm.Partition, ShardID: string(itm.Shard)}] = struct{}{}
	}
	*ss = ssn
	return nil
}

func newShardTechnicalRecord(validators []string) (certification.TechnicalRecord, error) {
	if len(validators) == 0 {
		return certification.TechnicalRecord{}, errors.New("validator list empty")
	}

	tr := certification.TechnicalRecord{
		Round:  1,
		Epoch:  0,
		Leader: validators[0],
		// precalculated hash of CBOR(certification.StatisticalRecord{})
		StatHash: []uint8{0x24, 0xee, 0x26, 0xf4, 0xaa, 0x45, 0x48, 0x5f, 0x53, 0xaa, 0xb4, 0x77, 0x57, 0xd0, 0xb9, 0x71, 0x99, 0xa3, 0xd9, 0x5f, 0x50, 0xcb, 0x97, 0x9c, 0x38, 0x3b, 0x7e, 0x50, 0x24, 0xf9, 0x21, 0xff},
	}

	fees := map[string]uint64{}
	for _, v := range validators {
		fees[v] = 0
	}
	h := hash.New(crypto.SHA256.New())
	h.WriteRaw(types.RawCBOR{0xA0}) // empty map
	h.Write(fees)

	var err error
	if tr.FeeHash, err = h.Sum(); err != nil {
		return tr, fmt.Errorf("calculating fee hash: %w", err)
	}

	return tr, nil
}
