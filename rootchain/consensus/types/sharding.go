package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type Orchestration interface {
	ShardEpoch(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error)
	ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error)
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

	if err = si.Init(pg); err != nil {
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
	tr.StatHash, err = si.statHash(crypto.SHA256)
	if err != nil {
		return nil, fmt.Errorf("calculating stat hash: %w", err)
	}
	// PrevEpochFees == CBOR(empty map), current == map[nodeID]->0
	tr.FeeHash, err = si.feeHash(crypto.SHA256)
	if err != nil {
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
	m         sync.Mutex
}

/*
Init sets up internal caches, calculated values etc which are not restored
automatically on deserialization.
*/
func (si *ShardInfo) Init(pg *genesis.GenesisPartitionRecord) error {
	if si.Leader == "" && len(pg.Nodes) > 0 {
		si.Leader = pg.Nodes[0].NodeIdentifier
	}

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

func (si *ShardInfo) NextEpoch(pg *genesis.GenesisPartitionRecord) (*ShardInfo, error) {
	si.m.Lock()
	defer si.m.Unlock()

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

	return nextSI, nextSI.Init(pg)
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

func (si *ShardInfo) TechnicalRecord(req *certification.BlockCertificationRequest, orc Orchestration) (tr certification.TechnicalRecord, err error) {
	si.m.Lock()
	defer si.m.Unlock()

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

func (si *ShardInfo) SetLatestCert(cert *certification.CertificationResponse) error {
	si.m.Lock()
	defer si.m.Unlock()
	si.LastCR = cert
	return nil
}

func (si *ShardInfo) ValidRequest(req *certification.BlockCertificationRequest) error {
	if err := si.Verify(req.NodeIdentifier, req.IsValid); err != nil {
		return fmt.Errorf("invalid certification request: %w", err)
	}

	si.m.Lock()
	defer si.m.Unlock()

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
	si.m.Lock()
	defer si.m.Unlock()
	// at least 50%
	return uint64(len(si.trustBase)/2) + 1
}

func (si *ShardInfo) GetTotalNodes() uint64 {
	si.m.Lock()
	defer si.m.Unlock()
	return uint64(len(si.trustBase))
}

func (si *ShardInfo) Verify(nodeID string, f func(v abcrypto.Verifier) error) error {
	si.m.Lock()
	defer si.m.Unlock()

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
