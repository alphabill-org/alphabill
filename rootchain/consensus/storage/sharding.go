package storage

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

type ShardStates struct {
	States  map[types.PartitionShardID]*ShardInfo
	Changed ShardSet // shards whose state has changed

	// cache schemes of the block
	schemes map[types.PartitionID]types.ShardingScheme
}

/*
nextBlock returns shard states for the next block based on the configs of the round - if the
shard was part of the current block it's state is cloned, otherwise new empty state is added.
*/
func (ss ShardStates) nextBlock(shardConfs map[types.PartitionShardID]*types.PartitionDescriptionRecord, hashAlg crypto.Hash) (nextBlock ShardStates, err error) {
	nextBlock.States = make(map[types.PartitionShardID]*ShardInfo, len(shardConfs))
	nextBlock.Changed = ShardSet{}
	for k, pdr := range shardConfs {
		if len(pdr.Validators) == 0 {
			continue //  shard is deleted
		}
		if prevSI, ok := ss.States[k]; ok {
			// prevSI.IR is the state in the shard when prevSI.TR was created - so when the epoch in the TR
			// is different the previous round triggered epoch change in the shard and this round should
			// switch the shard state into next epoch too
			if prevSI.TR.Epoch != prevSI.IR.Epoch {
				if nextBlock.States[k], err = prevSI.nextEpoch(pdr, hashAlg); err != nil {
					return nextBlock, fmt.Errorf("creating ShardInfo %s - %s of the next epoch: %w",
						prevSI.LastCR.Partition, prevSI.LastCR.Shard, err)
				}
			} else {
				si := *prevSI
				si.Fees = maps.Clone(si.Fees)
				nextBlock.States[k] = &si
			}
		} else {
			// must be new shard activated in this round
			if nextBlock.States[k], err = NewShardInfo(pdr, hashAlg); err != nil {
				return nextBlock, fmt.Errorf("creating ShardInfo for %s: %w", k, err)
			}
			nextBlock.Changed[k] = struct{}{}
		}
	}

	return nextBlock, nil
}

/*
UnicityTree builds the unicity tree based on the shard states.
*/
func (ss ShardStates) UnicityTree(algo crypto.Hash) (*types.UnicityTree, map[types.PartitionID]types.ShardTree, error) {
	schemes, err := ss.shardingSchemes()
	if err != nil {
		return nil, nil, fmt.Errorf("acquiring sharding schemes: %w", err)
	}
	utData := make([]*types.UnicityTreeData, 0, len(schemes))
	shardTrees := make(map[types.PartitionID]types.ShardTree)
	var si *ShardInfo
	var ok bool
	for partitionID, shards := range schemes {
		sti := make([]types.ShardTreeInput, 0, len(shards))
		for shardID := range shards.All() {
			psID := types.PartitionShardID{PartitionID: partitionID, ShardID: shardID.Key()}
			if si, ok = ss.States[psID]; !ok {
				return nil, nil, fmt.Errorf("missing shard info for %s", psID.String())
			}
			trHash, err := si.TR.Hash()
			if err != nil {
				return nil, nil, fmt.Errorf("calculating TR hash: %w", err)
			}
			sti = append(sti, types.ShardTreeInput{
				Shard:         shardID,
				IR:            si.IR,
				TRHash:        trHash,
				ShardConfHash: si.ShardConfHash,
			})
		}
		shardTree, err := types.CreateShardTree(shards, sti, algo)
		if err != nil {
			return nil, nil, fmt.Errorf("creating shard tree: %w", err)
		}

		shardTrees[partitionID] = shardTree
		utData = append(utData, &types.UnicityTreeData{
			Partition:     partitionID,
			ShardTreeRoot: shardTree.RootHash(),
		})
	}

	ut, err := types.NewUnicityTree(algo, utData)
	if err != nil {
		return nil, nil, err
	}
	return ut, shardTrees, nil
}

/*
certificationResponses builds the unicity tree and certification responses based on the shard states.
CertificationResponse will be generated only for shards marked as "changed".
The UnicityCertificates in the response are not complete, they miss the UnicitySeal.
*/
func (ss ShardStates) certificationResponses(algo crypto.Hash) ([]*certification.CertificationResponse, []byte, error) {
	ut, shardTrees, err := ss.UnicityTree(algo)
	if err != nil {
		return nil, nil, fmt.Errorf("creating unicity tree: %w", err)
	}

	crs := make([]*certification.CertificationResponse, 0, len(ss.Changed))
	for psID, si := range ss.changedShards() {
		stCert, err := shardTrees[psID.PartitionID].Certificate(si.ShardID)
		if err != nil {
			return nil, nil, fmt.Errorf("creating shard tree certificate: %w", err)
		}

		utCert, err := ut.Certificate(si.PartitionID)
		if err != nil {
			return nil, nil, fmt.Errorf("creating unicity tree certificate: %w", err)
		}

		trHash, err := si.TR.Hash()
		if err != nil {
			return nil, nil, fmt.Errorf("calculating TR hash: %w", err)
		}

		crs = append(crs, &certification.CertificationResponse{
			Partition: si.PartitionID,
			Shard:     si.ShardID,
			Technical: si.TR,
			UC: types.UnicityCertificate{
				Version:                1,
				InputRecord:            si.IR,
				TRHash:                 trHash,
				ShardConfHash:          si.ShardConfHash,
				UnicityTreeCertificate: utCert,
				ShardTreeCertificate:   stCert,
			},
		})
	}

	return crs, ut.RootHash(), err
}

func (ss *ShardStates) shardingSchemes() (map[types.PartitionID]types.ShardingScheme, error) {
	if ss.schemes != nil {
		return ss.schemes, nil
	}

	schemes := map[types.PartitionID]types.ShardingScheme{}
	for k, v := range ss.States {
		if v.ShardID.Length() == 0 {
			if _, ok := schemes[k.PartitionID]; ok {
				return nil, fmt.Errorf("invalid sharding scheme for partition %s - empty shardID in a multi shard scheme", k.PartitionID)
			}
			schemes[k.PartitionID] = types.ShardingScheme{}
			continue
		}
		schemes[k.PartitionID] = append(schemes[k.PartitionID], v.ShardID)
	}

	for partitionID, shards := range schemes {
		if err := shards.IsValid(); err != nil {
			return nil, fmt.Errorf("invalid sharding scheme for partition %s: %w", partitionID, err)
		}
	}

	ss.schemes = schemes
	return schemes, nil
}

/*
changedShards is iterator over shards which had ChangeRequest on the round.
*/
func (ss ShardStates) changedShards() iter.Seq2[types.PartitionShardID, *ShardInfo] {
	return func(yield func(types.PartitionShardID, *ShardInfo) bool) {
		for k := range ss.Changed {
			if !yield(k, ss.States[k]) {
				return
			}
		}
	}
}

/*
ssItems is helper type for serializing shardStates - maps with complex key are not
handled correctly by the CBOR lib so we serialize it as a array.
*/
type ssItems struct {
	_       struct{} `cbor:",toarray"`
	Version uint32
	Data    []*ShardInfo
	Changes ShardSet
}

func (ss ShardStates) MarshalCBOR() ([]byte, error) {
	d := ssItems{
		Version: 1,
		Data:    make([]*ShardInfo, len(ss.States)),
		Changes: ss.Changed,
	}
	idx := 0
	for _, si := range ss.States {
		d.Data[idx] = si
		idx++
	}
	buf, err := types.Cbor.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("serializing shard states: %w", err)
	}
	return buf, nil
}

func (ss *ShardStates) UnmarshalCBOR(data []byte) error {
	var d ssItems
	if err := types.Cbor.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("decoding shard states: %w", err)
	}
	if d.Version != 1 {
		return fmt.Errorf("unsupported shard data version %d", d.Version)
	}
	ssn := ShardStates{
		States:  make(map[types.PartitionShardID]*ShardInfo, len(d.Data)),
		Changed: d.Changes,
	}
	for _, itm := range d.Data {
		ssn.States[types.PartitionShardID{PartitionID: itm.PartitionID, ShardID: itm.ShardID.Key()}] = itm
	}
	*ss = ssn
	return nil
}

func NewShardInfo(shardConf *types.PartitionDescriptionRecord, hashAlg crypto.Hash) (*ShardInfo, error) {
	shardConfHash, err := shardConf.Hash(hashAlg)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shard conf hash: %w", err)
	}
	si := &ShardInfo{
		PartitionID:   shardConf.PartitionID,
		ShardID:       shardConf.ShardID,
		T2Timeout:     shardConf.T2Timeout,
		ShardConfHash: shardConfHash,
		RootHash:      nil,
		PrevEpochFees: types.RawCBOR{0xA0}, // CBOR map(0)
		LastCR:        nil,
		IR:            &types.InputRecord{Version: 1},
	}

	if si.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("init previous epoch stat: %w", err)
	}

	si.resetFeeList(shardConf)

	if err = si.resetTrustBase(shardConf); err != nil {
		return nil, fmt.Errorf("shard info init: %w", err)
	}

	if si.TR, err = newShardTechnicalRecord(shardConf, si.nodeIDs); err != nil {
		return nil, fmt.Errorf("failed to create technical record for shard %d-%s: %w", si.PartitionID, si.ShardID, err)
	}

	return si, nil
}

func newShardTechnicalRecord(shardConf *types.PartitionDescriptionRecord, validators []string) (certification.TechnicalRecord, error) {
	if len(validators) == 0 {
		return certification.TechnicalRecord{}, errors.New("validator list empty")
	}

	tr := certification.TechnicalRecord{
		Round:  1,
		Epoch:  shardConf.Epoch,
		Leader: validators[0],
		// precalculated hash of CBOR(certification.StatisticalRecord{})
		StatHash: []uint8{0x24, 0xee, 0x26, 0xf4, 0xaa, 0x45, 0x48, 0x5f, 0x53, 0xaa, 0xb4, 0x77, 0x57, 0xd0, 0xb9, 0x71, 0x99, 0xa3, 0xd9, 0x5f, 0x50, 0xcb, 0x97, 0x9c, 0x38, 0x3b, 0x7e, 0x50, 0x24, 0xf9, 0x21, 0xff},
	}

	fees := map[string]uint64{}
	for _, v := range validators {
		fees[v] = 0
	}
	h := abhash.New(crypto.SHA256.New())
	h.WriteRaw(types.RawCBOR{0xA0}) // empty map
	h.Write(fees)

	var err error
	if tr.FeeHash, err = h.Sum(); err != nil {
		return tr, fmt.Errorf("calculating fee hash: %w", err)
	}

	return tr, nil
}

type ShardInfo struct {
	_             struct{} `cbor:",toarray"`
	PartitionID   types.PartitionID
	ShardID       types.ShardID
	T2Timeout     time.Duration
	ShardConfHash []byte
	RootHash      []byte // last certified root hash

	// statistical record of the previous epoch. As we only need
	// it for hashing we keep it in serialized representation
	PrevEpochStat types.RawCBOR

	// statistical record of the current epoch
	Stat certification.StatisticalRecord

	// per validator total, invariant fees of the previous epoch
	// but as with statistical record of the previous epoch we need it
	// for hashing so we keep it in serialized representation
	PrevEpochFees types.RawCBOR

	Fees map[string]uint64 // per validator summary fees of the current epoch

	LastCR *certification.CertificationResponse // last response sent to shard

	// latest change request data for creating next certificate
	IR *types.InputRecord
	TR certification.TechnicalRecord

	nodeIDs   []string // sorted list of partition node IDs
	trustBase map[string]abcrypto.Verifier
}

func (si *ShardInfo) resetFeeList(shardConf *types.PartitionDescriptionRecord) {
	fees := make(map[string]uint64)
	for _, n := range shardConf.Validators {
		fees[n.NodeID] = 0
	}
	si.Fees = fees
}

func (si *ShardInfo) resetTrustBase(shardConf *types.PartitionDescriptionRecord) error {
	si.nodeIDs = make([]string, 0, len(shardConf.Validators))
	si.trustBase = make(map[string]abcrypto.Verifier)
	for _, v := range shardConf.Validators {
		ver, err := v.SigVerifier()
		if err != nil {
			return fmt.Errorf("creating verifier for validator %q: %w", v.NodeID, err)
		}
		si.trustBase[v.NodeID] = ver
		si.nodeIDs = append(si.nodeIDs, v.NodeID)
	}
	slices.Sort(si.nodeIDs)

	return si.IsValid()
}

func (si *ShardInfo) IsValid() error {
	if n := len(si.Fees); n != len(si.nodeIDs) {
		return fmt.Errorf("shard has %d nodes but fee list contains %d nodes", len(si.nodeIDs), n)
	}
	if si.LastCR != nil {
		if err := si.LastCR.IsValid(); err != nil {
			return fmt.Errorf("last certification response is invalid: %w", err)
		}
	}
	return nil
}

func (si *ShardInfo) nextEpoch(shardConf *types.PartitionDescriptionRecord, hashAlg crypto.Hash) (*ShardInfo, error) {
	if si.TR.Epoch != shardConf.Epoch {
		return nil, fmt.Errorf("epochs must be consecutive, expected %d proposed next %d", si.TR.Epoch, shardConf.Epoch)
	}
	shardConfHash, err := shardConf.Hash(hashAlg)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shard conf hash: %w", err)
	}
	nextSI := &ShardInfo{
		PartitionID:   shardConf.PartitionID,
		ShardID:       shardConf.ShardID,
		T2Timeout:     shardConf.T2Timeout,
		ShardConfHash: shardConfHash,
		RootHash:      si.RootHash,
		LastCR:        si.LastCR,
		IR:            si.IR,
		TR:            si.TR,
	}

	if nextSI.PrevEpochFees, err = types.Cbor.Marshal(si.Fees); err != nil {
		return nil, fmt.Errorf("encoding previous epoch fees: %w", err)
	}
	if nextSI.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("encoding previous epoch stat: %w", err)
	}

	nextSI.resetFeeList(shardConf)
	if err = nextSI.resetTrustBase(shardConf); err != nil {
		return nil, fmt.Errorf("initializing shard trustbase: %w", err)
	}

	return nextSI, nil
}

/*
nextRound advances the TR of the ShardInfo into the next round, ready to be used for
generating CertificationResponse. The "pdr" argument must be the shard configuration
for the next round.
*/
func (si *ShardInfo) nextRound(req *certification.BlockCertificationRequest, pdr *types.PartitionDescriptionRecord, hashAlg crypto.Hash) (err error) {
	// timeout IRChangeRequest doesn't have BlockCertificationRequest
	if req != nil {
		si.update(req, si.TR.Leader)
	}

	nextShardInfo := si
	if si.TR.Epoch != pdr.Epoch {
		si.TR.Epoch++
		if nextShardInfo, err = si.nextEpoch(pdr, hashAlg); err != nil {
			return fmt.Errorf("creating ShardInfo of the next epoch: %w", err)
		}
	}

	if si.TR.FeeHash, err = nextShardInfo.feeHash(crypto.SHA256); err != nil {
		return fmt.Errorf("hashing fees: %w", err)
	}
	if si.TR.StatHash, err = nextShardInfo.statHash(crypto.SHA256); err != nil {
		return fmt.Errorf("hashing statistics: %w", err)
	}
	si.TR.Round++
	si.TR.Leader = nextShardInfo.selectLeader(si.TR.Round)

	return nil
}

/*
StatHash returns hash of the StatisticalRecord, suitable
for use in Technical Record.
*/
func (si *ShardInfo) statHash(algo crypto.Hash) ([]byte, error) {
	h := abhash.New(algo.New())
	h.WriteRaw(si.PrevEpochStat)
	h.Write(si.Stat)
	return h.Sum()
}

func (si *ShardInfo) feeHash(algo crypto.Hash) ([]byte, error) {
	h := abhash.New(algo.New())
	h.WriteRaw(si.PrevEpochFees)
	h.Write(si.Fees)
	return h.Sum()
}

func (si *ShardInfo) update(req *certification.BlockCertificationRequest, leader string) {
	si.RootHash = req.InputRecord.Hash
	si.Fees[leader] += req.InputRecord.SumOfEarnedFees

	if !bytes.Equal(req.InputRecord.Hash, req.InputRecord.PreviousHash) {
		si.Stat.Blocks += 1
	}
	si.Stat.BlockFees += req.InputRecord.SumOfEarnedFees
	si.Stat.BlockSize += req.BlockSize
	si.Stat.StateSize += req.StateSize
	si.Stat.MaxFee = max(si.Stat.MaxFee, req.InputRecord.SumOfEarnedFees)
	si.Stat.MaxBlockSize = max(si.Stat.MaxBlockSize, req.BlockSize)
	si.Stat.MaxStateSize = max(si.Stat.MaxStateSize, req.StateSize)
}

func (si *ShardInfo) ValidRequest(req *certification.BlockCertificationRequest) error {
	// req.IsValid checks that req is not nil and comes from valid validator.
	// It also calls IR.IsValid which implements (CR.IR.h = CR.IR.h′) = (CR.IR.hB = ⊥)
	// check so we do not repeat it here.
	if err := si.Verify(req.NodeID, req.IsValid); err != nil {
		return fmt.Errorf("invalid certification request: %w", err)
	}

	if req.PartitionID != si.PartitionID || !req.ShardID.Equal(si.ShardID) {
		return fmt.Errorf("request of shard %s-%s but ShardInfo of %s-%s", req.PartitionID, req.ShardID, si.PartitionID, si.ShardID)
	}

	if req.IRRound() != si.LastCR.Technical.Round {
		return fmt.Errorf("expected round %d, got %d", si.LastCR.Technical.Round, req.IRRound())
	}
	if req.InputRecord.Epoch != si.LastCR.Technical.Epoch {
		return fmt.Errorf("expected epoch %d, got %d", si.LastCR.Technical.Epoch, req.InputRecord.Epoch)
	}

	if !bytes.Equal(req.IRPreviousHash(), si.RootHash) {
		return errors.New("request has different root hash for last certified state")
	}

	// CR.IR.t = SI[β,σ].UC_.Cr.t – time reference is equal to the time field of the previous unicity seal
	if req.InputRecord.Timestamp != si.LastCR.UC.UnicitySeal.Timestamp {
		return fmt.Errorf("IR timestamp %d doesn't match UnicitySeal timestamp %d", req.InputRecord.Timestamp, si.LastCR.UC.UnicitySeal.Timestamp)
	}

	return nil
}

func (si *ShardInfo) GetQuorum() uint64 {
	// at least 50%
	return (uint64(len(si.trustBase)) / 2) + 1
}

func (si *ShardInfo) GetTotalNodes() uint64 {
	return uint64(len(si.trustBase))
}

func (si *ShardInfo) Verify(nodeID string, f func(v abcrypto.Verifier) error) error {
	if v, ok := si.trustBase[nodeID]; ok {
		return f(v)
	}
	return fmt.Errorf("node %q is not in the trustbase of the shard", nodeID)
}

func (si *ShardInfo) selectLeader(seed uint64) string {
	// TODO: if rootHash is nil for new shard, leader is predictable
	extra := si.RootHash
	if len(extra) >= 4 {
		seed += (uint64(extra[0]) | uint64(extra[1])<<8 | uint64(extra[2])<<16 | uint64(extra[3])<<24)
	}
	peerCount := uint64(len(si.nodeIDs))

	return si.nodeIDs[seed%peerCount]
}

type (
	ShardSet map[types.PartitionShardID]struct{}

	/*
	   shardSetItem is helper type for serializing ShardSet - map with complex key
	   is not handled properly by the CBOR library so we serialize it as array.
	*/
	shardSetItem struct {
		_         struct{} `cbor:",toarray"`
		Partition types.PartitionID
		Shard     []byte
	}
)

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
