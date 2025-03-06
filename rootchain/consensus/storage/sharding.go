package storage

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

type shardStates map[types.PartitionShardID]*ShardInfo

/*
nextBlock returns shard states for the next block by "cloning" the current states.
*/
func (ss shardStates) nextBlock(parentIRs InputRecords, orchestration Orchestration, rootRound uint64, hashAlg crypto.Hash) (shardStates, error) {
	nextBlock := make(shardStates, len(ss))

	shardConfs, err := orchestration.ShardConfigs(rootRound)
	if err != nil {
		return nil, fmt.Errorf("loading shard configurations for round %d: %w", rootRound, err)
	}

	for shardKey, prevSI := range ss {
		parentIR := parentIRs.Find(prevSI.PartitionID, prevSI.ShardID)

		// There should be a parent IR, unless it's a newly activated shard at round 0
		if parentIR == nil && parentIR.Technical.Round > 0 {
			return nil, fmt.Errorf("no previous round data for shard %s - %s", prevSI.LastCR.Partition, prevSI.LastCR.Shard)
		}

		// parentIR.IR is the state in the shard when parentIR.Technical was created - so when the epoch
		// is different this new block is in the new epoch
		if parentIR != nil && parentIR.Technical.Epoch != parentIR.IR.Epoch {
			shardConf, ok := shardConfs[shardKey]
			if !ok {
				return nil, fmt.Errorf("shard %s missing configuration", shardKey)
			}
			if parentIR.Technical.Epoch != shardConf.ShardEpoch {
				return nil, fmt.Errorf("shard %s expected epoch %d, loaded configuration has epoch %d",
					shardKey, parentIR.Technical.Epoch, shardConf.ShardEpoch)
			}
			if nextBlock[shardKey], err = prevSI.nextEpoch(shardConf.ShardEpoch, shardConf, hashAlg); err != nil {
				return nil, fmt.Errorf("creating ShardInfo %s - %s of the next epoch: %w",
					prevSI.LastCR.Partition, prevSI.LastCR.Shard, err)
			}
		} else {
			si := *prevSI
			si.Fees = maps.Clone(si.Fees)
			nextBlock[shardKey] = &si
		}
	}

	// Do we have new shards activated in this round?
	for shardKey, shardConf := range shardConfs {
		if _, ok := nextBlock[shardKey]; ok {
			// not a new shard
			continue
		}
		si, err := NewShardInfo(shardConf, hashAlg)
		if err != nil {
			return nil, fmt.Errorf("creating ShardInfo for partition %s: %w", shardConf.PartitionID, err)
		}
		nextBlock[shardKey] = si
	}

	return nextBlock, nil
}

/*
ssItems is helper type for serializing shardStates - maps with complex key are not
handled correctly by the CBOR lib so we serialize it as a array.
*/
type ssItems struct {
	_       struct{} `cbor:",toarray"`
	Version uint32
	Data    []*ShardInfo
}

func (ss shardStates) MarshalCBOR() ([]byte, error) {
	d := ssItems{
		Version: 1,
		Data:    make([]*ShardInfo, len(ss)),
	}
	idx := 0
	for _, si := range ss {
		d.Data[idx] = si
		idx++
	}
	buf, err := types.Cbor.Marshal(d)
	if err != nil {
		return nil, fmt.Errorf("serializing shard states: %w", err)
	}
	return buf, nil
}

func (ss *shardStates) UnmarshalCBOR(data []byte) error {
	var d ssItems
	if err := types.Cbor.Unmarshal(data, &d); err != nil {
		return fmt.Errorf("decoding shard states: %w", err)
	}
	if d.Version != 1 {
		return fmt.Errorf("unsupported shard data version %d", d.Version)
	}
	ssn := make(shardStates, len(d.Data))
	for _, itm := range d.Data {
		ssn[types.PartitionShardID{PartitionID: itm.LastCR.Partition, ShardID: itm.LastCR.Shard.Key()}] = itm
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
	}

	if si.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("init previous epoch stat: %w", err)
	}

	si.resetFeeList(shardConf)
	if err = si.resetTrustBase(shardConf); err != nil {
		return nil, fmt.Errorf("shard info init: %w", err)
	}

	return si, nil
}

type ShardInfo struct {
	_        struct{} `cbor:",toarray"`
	PartitionID   types.PartitionID
	ShardID       types.ShardID
	T2Timeout     time.Duration
	ShardConfHash []byte

	RootHash      []byte   // last certified root hash
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

func (si *ShardInfo) nextEpoch(shardEpoch uint64, shardConf *types.PartitionDescriptionRecord, hashAlg crypto.Hash) (*ShardInfo, error) {
	if shardEpoch != shardConf.ShardEpoch {
		return nil, fmt.Errorf("epochs must be consecutive, expected %d proposed next %d", shardEpoch, shardConf.ShardEpoch)
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

func (si *ShardInfo) nextRound(req *certification.BlockCertificationRequest, lastTR certification.TechnicalRecord, orc Orchestration, rootRound uint64, hashAlg crypto.Hash) (tr certification.TechnicalRecord, err error) {
	// timeout IRChangeRequest doesn't have BlockCertificationRequest
	if req != nil {
		si.update(req, lastTR.Leader)
	}

	tr.Round = lastTR.Round + 1

	nextShardInfo := si
	// New shard epoch is activated for the next shard round, if current root round
	// reaches the activation root round of the shard configuration.
	nextShardConf, err := orc.ShardConfig(si.PartitionID, si.ShardID, rootRound)
	if err != nil {
		return tr, fmt.Errorf("reading config of the epoch: %w", err)
	}

	tr.Epoch = nextShardConf.ShardEpoch
	if lastTR.Epoch != tr.Epoch {
		if nextShardInfo, err = si.nextEpoch(lastTR.Epoch+1, nextShardConf, hashAlg); err != nil {
			return tr, fmt.Errorf("creating ShardInfo of the next epoch: %w", err)
		}
	}

	if tr.FeeHash, err = nextShardInfo.feeHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing fees: %w", err)
	}
	if tr.StatHash, err = nextShardInfo.statHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing statistics: %w", err)
	}
	tr.Leader = nextShardInfo.selectLeader(tr.Round)

	return tr, nil
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

	if req.PartitionID != si.LastCR.Partition || !req.ShardID.Equal(si.LastCR.Shard) {
		return fmt.Errorf("request of shard %s-%s but ShardInfo of %s-%s", req.PartitionID, req.ShardID, si.LastCR.Partition, si.LastCR.Shard)
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
	return uint64(len(si.trustBase)/2) + 1
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
	// TODO: if rootHash is nil for new shard, leader is predicatble
	extra := si.RootHash
	if len(extra) >= 4 {
		seed += (uint64(extra[0]) | uint64(extra[1])<<8 | uint64(extra[2])<<16 | uint64(extra[3])<<24)
	}
	peerCount := uint64(len(si.nodeIDs))

	return si.nodeIDs[seed%peerCount]
}
