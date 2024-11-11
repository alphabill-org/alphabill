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
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type shardStates map[partitionShard]*ShardInfo

/*
nextBlock returns shard states for the next block by "cloning" the current states.
*/
func (ss shardStates) nextBlock(orchestration Orchestration) (shardStates, error) {
	nextBlock := make(shardStates, len(ss))
	for k, v := range ss {
		if v.Epoch != v.LastCR.Technical.Epoch {
			pg, err := orchestration.ShardConfig(v.LastCR.Partition, v.LastCR.Shard, v.LastCR.Technical.Epoch)
			if err != nil {
				return nil, fmt.Errorf("loading shard config: %w", err)
			}
			if nextBlock[k], err = v.nextEpoch(pg); err != nil {
				return nil, fmt.Errorf("creating ShardInfo %s - %s of the next epoch: %w", v.LastCR.Partition, v.LastCR.Shard, err)
			}
		} else {
			si := *v
			si.Fees = maps.Clone(si.Fees)
			nextBlock[k] = &si
		}
	}
	return nextBlock, nil
}

/*
init calls init for all the ShardInfo instances in the map
*/
func (ss shardStates) init(orchestration Orchestration) error {
	for _, si := range ss {
		pg, err := orchestration.ShardConfig(si.LastCR.Partition, si.LastCR.Shard, si.Epoch)
		if err != nil {
			return fmt.Errorf("acquiring shard configuration: %w", err)
		}
		if err = si.init(pg); err != nil {
			return fmt.Errorf("init shard info (%s - %s): %w", si.LastCR.Partition, si.LastCR.Shard, err)
		}
	}
	return nil
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
		Round:         pg.Certificate.InputRecord.RoundNumber,
		Epoch:         pg.Certificate.InputRecord.Epoch,
		RootHash:      pg.Certificate.InputRecord.Hash,
		PrevEpochFees: types.RawCBOR{0xA0}, // CBOR map(0)
		LastCR: &certification.CertificationResponse{
			Partition: pg.PartitionDescription.PartitionIdentifier,
			Shard:     types.ShardID{},
			UC:        *pg.Certificate,
		},
	}

	var err error
	if si.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("init previous epoch stat: %w", err)
	}

	if err = si.init(pg); err != nil {
		return nil, fmt.Errorf("shard info init: %w", err)
	}

	tr := certification.TechnicalRecord{
		Round: si.Round + 1,
		Epoch: si.Epoch,
		// we do not have UC of the previous round (used as input for leader selection)
		// so we just start with the first validator in the list. This should only happen
		// for bootstrap genesis, later we do have UC_ and select leader based on that
		Leader: pg.Nodes[0].NodeIdentifier,
	}
	// PrevEpochStat == current == zero value of stat record
	if tr.StatHash, err = si.statHash(crypto.SHA256); err != nil {
		return nil, fmt.Errorf("calculating stat hash: %w", err)
	}
	// PrevEpochFees == CBOR(empty map), current == map[nodeID]->0
	if tr.FeeHash, err = si.feeHash(crypto.SHA256); err != nil {
		return nil, fmt.Errorf("calculating fee hash: %w", err)
	}
	if err = si.LastCR.SetTechnicalRecord(tr); err != nil {
		return nil, fmt.Errorf("setting TechnicalRecord: %w", err)
	}

	return si, nil
}

type ShardInfo struct {
	_        struct{} `cbor:",toarray"`
	Round    uint64
	Epoch    uint64
	RootHash []byte // last certified root hash

	// statistical record of the previous epoch. As we only need
	// it for hashing we keep it in serialized representation
	PrevEpochStat types.RawCBOR

	// statistical record of the current epoch
	Stat certification.StatisticalRecord

	// per validator total, invariant fees of the previous epoch
	// but as with statistical record of the previous epoch we need it
	// for hashing so we keep it in serialized representation
	PrevEpochFees types.RawCBOR

	Fees   map[string]uint64 // per validator summary fees of the current epoch
	Leader string            // identifier of the Round leader

	LastCR *certification.CertificationResponse // last response sent to shard

	nodeIDs   []string // sorted list of partition node IDs
	trustBase map[string]abcrypto.Verifier
}

/*
init sets up internal caches, calculated values etc which are not restored
automatically on deserialization.
*/
func (si *ShardInfo) init(pg *genesis.GenesisPartitionRecord) error {
	// TODO: genesis must support sharding - currently we only support
	// single shard partitions so all nodes belong into the same shard!
	fees := make(map[string]uint64)
	for _, id := range pg.Nodes {
		fees[id.NodeIdentifier] = 0
	}
	si.Fees = fees

	// cache list of sorted node IDs for selecting leader
	nodes := slices.Collect(maps.Keys(si.Fees))
	slices.Sort(nodes)
	si.nodeIDs = nodes
	if len(nodes) == 0 {
		return errors.New("no validators in the fee list")
	}

	// TODO: genesis must support sharding - currently we only support
	// single shard partitions so all nodes belong into the same shard!
	si.trustBase = make(map[string]abcrypto.Verifier)
	for _, node := range pg.Nodes {
		ver, err := abcrypto.NewVerifierSecp256k1(node.SigningPublicKey)
		if err != nil {
			return fmt.Errorf("creating verifier for the node %q: %w", node.NodeIdentifier, err)
		}
		si.trustBase[node.NodeIdentifier] = ver
	}

	// this should only needed for genesis round
	if si.Leader == "" && len(pg.Nodes) > 0 {
		si.Leader = pg.Nodes[0].NodeIdentifier
	}

	return si.IsValid()
}

func (si *ShardInfo) IsValid() error {
	if si.Leader == "" {
		return errors.New("leader is unassigned")
	}
	if n := len(si.Fees); n != len(si.nodeIDs) {
		return fmt.Errorf("shard has %d nodes but fee list contains %d nodes", len(si.nodeIDs), n)
	}
	if si.LastCR == nil {
		return errors.New("last certification response is unassigned")
	}
	return nil
}

func (si *ShardInfo) nextEpoch(pg *genesis.GenesisPartitionRecord) (*ShardInfo, error) {
	if nextEpoch := pg.Certificate.InputRecord.Epoch; nextEpoch != si.Epoch+1 {
		return nil, fmt.Errorf("epochs must be consecutive, current is %d proposed next %d", si.Epoch, nextEpoch)
	}
	nextSI := &ShardInfo{
		Round:    si.Round,
		Epoch:    pg.Certificate.InputRecord.Epoch,
		RootHash: si.RootHash,
		Leader:   si.LastCR.Technical.Leader,
		LastCR:   si.LastCR,
	}

	var err error
	if nextSI.PrevEpochFees, err = types.Cbor.Marshal(si.Fees); err != nil {
		return nil, fmt.Errorf("encoding previous epoch fees: %w", err)
	}
	if nextSI.PrevEpochStat, err = types.Cbor.Marshal(si.Stat); err != nil {
		return nil, fmt.Errorf("encoding previous epoch stat: %w", err)
	}

	return nextSI, nextSI.init(pg)
}

func (si *ShardInfo) nextRound(req *certification.BlockCertificationRequest, orc Orchestration) (tr certification.TechnicalRecord, err error) {
	// timeout IRChangeRequest doesn't have BlockCertificationRequest
	if req != nil {
		si.update(req)
	}
	tr.Round = si.Round + 1

	if tr.FeeHash, err = si.feeHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing fees: %w", err)
	}
	if tr.StatHash, err = si.statHash(crypto.SHA256); err != nil {
		return tr, fmt.Errorf("hashing statistics: %w", err)
	}

	tr.Epoch, err = orc.ShardEpoch(si.LastCR.Partition, si.LastCR.Shard, tr.Round)
	if err != nil {
		return tr, fmt.Errorf("querying shard's epoch: %w", err)
	}
	if si.Epoch != tr.Epoch {
		pg, err := orc.ShardConfig(si.LastCR.Partition, si.LastCR.Shard, tr.Epoch)
		if err != nil {
			return tr, fmt.Errorf("reading config of the epoch: %w", err)
		}
		nesi, err := NewShardInfoFromGenesis(pg)
		if err != nil {
			return tr, err
		}
		nesi.LastCR = si.LastCR
		tr.Leader = nesi.selectLeader()
	} else {
		tr.Leader = si.selectLeader()
	}

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

func (si *ShardInfo) update(req *certification.BlockCertificationRequest) {
	si.Round = req.IRRound()
	si.RootHash = req.InputRecord.Hash
	si.Fees[si.Leader] += req.InputRecord.SumOfEarnedFees

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
	if err := si.Verify(req.NodeIdentifier, req.IsValid); err != nil {
		return fmt.Errorf("invalid certification request: %w", err)
	}

	if req.IRRound() != si.Round+1 {
		return fmt.Errorf("expected round %d, got %d", si.Round+1, req.IRRound())
	}
	if req.InputRecord.Epoch != si.Epoch {
		return fmt.Errorf("expected epoch %d, got %d", si.Epoch, req.InputRecord.Epoch)
	}
	if !bytes.Equal(req.IRPreviousHash(), si.RootHash) {
		return errors.New("request has different root hash for last certified state")
	}
	if req.RootRound() != si.LastCR.UC.GetRootRoundNumber() {
		return fmt.Errorf("request root round number %v does not match LUC root round %v", req.RootRound(), si.LastCR.UC.GetRootRoundNumber())
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

func (si *ShardInfo) selectLeader() string {
	leaderSeed := si.Round
	if si.LastCR != nil {
		extra := si.LastCR.UC.Hash(crypto.SHA256)
		leaderSeed += (uint64(extra[0]) | uint64(extra[1])<<8 | uint64(extra[2])<<16 | uint64(extra[3])<<24)
	}
	peerCount := uint64(len(si.nodeIDs))

	return si.nodeIDs[leaderSeed%peerCount]
}
