package storage

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"maps"
	"slices"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	rcgenesis "github.com/alphabill-org/alphabill/rootchain/genesis"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
)

type shardStates map[partitionShard]*ShardInfo

/*
nextBlock returns shard states for the next block by "cloning" the current states.
*/
func (ss shardStates) nextBlock(parentIR InputRecords, orchestration Orchestration) (shardStates, error) {
	nextBlock := make(shardStates, len(ss))
	for shardKey, prevSI := range ss {
		irData := parentIR.Find(prevSI.LastCR.Partition)
		if irData == nil {
			return nil, fmt.Errorf("no previous round data for shard %s - %s", prevSI.LastCR.Partition, prevSI.LastCR.Shard)
		}
		// irData.Technical is the TR parent block created (or inherited) so the Round there is the
		// round we're creating new block for.
		epoch, err := orchestration.ShardEpoch(prevSI.LastCR.Partition, prevSI.LastCR.Shard, irData.Technical.Round)
		if err != nil {
			return nil, fmt.Errorf("querying shard epoch: %w", err)
		}
		// irData.IR is the state in the shard when irData.Technical was created - so when the epoch
		// is different this new block is in the new epoch
		if epoch != irData.IR.Epoch {
			rec, err := orchestration.ShardConfig(prevSI.LastCR.Partition, prevSI.LastCR.Shard, epoch)
			if err != nil {
				return nil, fmt.Errorf("loading shard config: %w", err)
			}
			if nextBlock[shardKey], err = prevSI.nextEpoch(epoch, rec); err != nil {
				return nil, fmt.Errorf("creating ShardInfo %s - %s of the next epoch: %w", prevSI.LastCR.Partition, prevSI.LastCR.Shard, err)
			}
		} else {
			si := *prevSI
			si.Fees = maps.Clone(si.Fees)
			nextBlock[shardKey] = &si
		}
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
		ssn[partitionShard{itm.LastCR.Partition, itm.LastCR.Shard.Key()}] = itm
	}
	*ss = ssn
	return nil
}

func NewShardInfoFromGenesis(pg *genesis.GenesisPartitionRecord) (*ShardInfo, error) {
	si := &ShardInfo{
		RootHash:      pg.Certificate.InputRecord.Hash,
		PrevEpochFees: types.RawCBOR{0xA0}, // CBOR map(0)
		LastCR: &certification.CertificationResponse{
			Partition: pg.PartitionDescription.PartitionID,
			Shard:     types.ShardID{},
			UC:        *pg.Certificate,
		},
	}

	var err error
	if si.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("init previous epoch stat: %w", err)
	}

	nodeIDs := util.TransformSlice(pg.Nodes, func(pn *genesis.PartitionNode) string { return pn.NodeID })
	tr, err := rcgenesis.TechnicalRecord(pg.Certificate.InputRecord, nodeIDs)
	if err != nil {
		return nil, fmt.Errorf("creating TechnicalRecord: %w", err)
	}
	if err = si.LastCR.SetTechnicalRecord(tr); err != nil {
		return nil, fmt.Errorf("setting TechnicalRecord: %w", err)
	}

	rec := partitions.NewVARFromGenesis(pg)
	si.resetFeeList(rec)
	if err = si.resetTrustBase(rec); err != nil {
		return nil, fmt.Errorf("shard info init: %w", err)
	}

	return si, nil
}

type ShardInfo struct {
	_        struct{} `cbor:",toarray"`
	RootHash []byte   // last certified root hash

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

func (si *ShardInfo) resetFeeList(rec *partitions.ValidatorAssignmentRecord) {
	fees := make(map[string]uint64)
	for _, n := range rec.Nodes {
		fees[n.NodeID] = 0
	}
	si.Fees = fees
}

func (si *ShardInfo) resetTrustBase(rec *partitions.ValidatorAssignmentRecord) error {
	// TODO: genesis must support sharding - currently we only support
	// single shard partitions so all nodes belong into the same shard!
	si.nodeIDs = make([]string, 0, len(rec.Nodes))
	si.trustBase = make(map[string]abcrypto.Verifier)
	for _, n := range rec.Nodes {
		ver, err := abcrypto.NewVerifierSecp256k1(n.SignKey)
		if err != nil {
			return fmt.Errorf("creating verifier for the node %q: %w", n.NodeID, err)
		}
		si.trustBase[n.NodeID] = ver
		si.nodeIDs = append(si.nodeIDs, n.NodeID)
	}
	slices.Sort(si.nodeIDs)

	return si.IsValid()
}

func (si *ShardInfo) IsValid() error {
	if n := len(si.Fees); n != len(si.nodeIDs) {
		return fmt.Errorf("shard has %d nodes but fee list contains %d nodes", len(si.nodeIDs), n)
	}
	if err := si.LastCR.IsValid(); err != nil {
		return fmt.Errorf("last certification response is invalid: %w", err)
	}
	return nil
}

func (si *ShardInfo) nextEpoch(epoch uint64, rec *partitions.ValidatorAssignmentRecord) (*ShardInfo, error) {
	if rec.EpochNumber != epoch {
		return nil, fmt.Errorf("epochs must be consecutive, expected %d proposed next %d", epoch, rec.EpochNumber)
	}
	nextSI := &ShardInfo{
		RootHash: si.RootHash,
		LastCR:   si.LastCR,
	}

	var err error
	if nextSI.PrevEpochFees, err = types.Cbor.Marshal(si.Fees); err != nil {
		return nil, fmt.Errorf("encoding previous epoch fees: %w", err)
	}
	if nextSI.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("encoding previous epoch stat: %w", err)
	}

	nextSI.resetFeeList(rec)
	if err = nextSI.resetTrustBase(rec); err != nil {
		return nil, fmt.Errorf("initializing shard trustbase: %w", err)
	}

	return nextSI, nil
}

func (si *ShardInfo) nextRound(req *certification.BlockCertificationRequest, lastTR certification.TechnicalRecord, orc Orchestration) (tr certification.TechnicalRecord, err error) {
	// timeout IRChangeRequest doesn't have BlockCertificationRequest
	if req != nil {
		si.update(req, lastTR.Leader)
	}

	tr.Round = lastTR.Round + 1

	siTR := si
	tr.Epoch, err = orc.ShardEpoch(si.LastCR.Partition, si.LastCR.Shard, tr.Round)
	if err != nil {
		return tr, fmt.Errorf("querying shard's epoch: %w", err)
	}
	if lastTR.Epoch != tr.Epoch {
		rec, err := orc.ShardConfig(si.LastCR.Partition, si.LastCR.Shard, tr.Epoch)
		if err != nil {
			return tr, fmt.Errorf("reading config of the epoch: %w", err)
		}
		if siTR, err = si.nextEpoch(lastTR.Epoch+1, rec); err != nil {
			return tr, fmt.Errorf("creating ShardInfo of the next epoch: %w", err)
		}
	}

	if tr.FeeHash, err = siTR.feeHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing fees: %w", err)
	}
	if tr.StatHash, err = siTR.statHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing statistics: %w", err)
	}
	tr.Leader = siTR.selectLeader(tr.Round)

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
	extra := si.RootHash
	seed += (uint64(extra[0]) | uint64(extra[1])<<8 | uint64(extra[2])<<16 | uint64(extra[3])<<24)
	peerCount := uint64(len(si.nodeIDs))

	return si.nodeIDs[seed%peerCount]
}
