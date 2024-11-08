package storage

import (
	"bytes"
	"cmp"
	"crypto"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type (
	BlockStore struct {
		hash          crypto.Hash // hash algorithm
		blockTree     *BlockTree
		storage       keyvaluedb.KeyValueDB
		shardInfo     map[partitionShard]*drctypes.ShardInfo
		orchestration Orchestration
		lock          sync.RWMutex
	}

	partitionShard struct {
		partition types.PartitionID
		shard     string // types.ShardID is not comparable
	}

	Orchestration interface {
		ShardEpoch(partition types.PartitionID, shard types.ShardID, round uint64) (uint64, error)
		ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*genesis.GenesisPartitionRecord, error)
	}
)

func storeGenesisInit(hash crypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) error {
	for _, genRec := range pg {
		partition := genRec.PartitionDescription.PartitionIdentifier
		for shard := range genRec.PartitionDescription.Shards.All() {
			si, err := drctypes.NewShardInfoFromGenesis(genRec)
			if err != nil {
				return fmt.Errorf("initializing shard info from genesis: %w", err)
			}
			// init round leader to first node in the shard genesis so that we
			// do not have to special case bootstrap round. When we get CertReq
			// for the "first real round" we assign fees (which is zero) of the
			// previous (nonexisting) round to this node...
			// This func is called only to store initial genesis so should be OK.
			si.Leader = pg[0].Nodes[0].NodeIdentifier

			if err := db.Write(certKey(partition, shard), si); err != nil {
				return fmt.Errorf("storing shard info: %w", err)
			}
		}
	}
	genesisBlock, err := NewGenesisBlock(hash, pg)
	if err != nil {
		return fmt.Errorf("creating genesis block: %w", err)
	}
	if err := blockStoreGenesisInit(genesisBlock, db); err != nil {
		return fmt.Errorf("storing genesis block: %w", err)
	}
	return nil
}

func readCertificates(db keyvaluedb.KeyValueDB, pg []*genesis.GenesisPartitionRecord) (map[partitionShard]*drctypes.ShardInfo, error) {
	ucs := make(map[partitionShard]*drctypes.ShardInfo)
	for _, genRec := range pg {
		partition := genRec.PartitionDescription.PartitionIdentifier
		for shard := range genRec.PartitionDescription.Shards.All() {
			var si drctypes.ShardInfo
			ok, err := db.Read(certKey(partition, shard), &si)
			if err != nil {
				return nil, fmt.Errorf("reading ShardInfo from storage: %w", err)
			}
			if !ok {
				return nil, fmt.Errorf("shard info {%s - %s} not found", partition, shard)
			}
			if err = si.Init(genRec); err != nil {
				return nil, fmt.Errorf("init shard info {%s - %s}: %w", partition, shard, err)
			}
			ucs[partitionShard{partition: partition, shard: shard.Key()}] = &si
		}
	}
	return ucs, nil
}

func New(hash crypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB, orchestration Orchestration) (block *BlockStore, err error) {
	// Initiate store
	if pg == nil {
		return nil, errors.New("genesis record is nil")
	}
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	// First start, initiate from genesis data
	empty, err := keyvaluedb.IsEmpty(db)
	if err != nil {
		return nil, fmt.Errorf("failed to read block store: %w", err)
	}
	if empty {
		if err = storeGenesisInit(hash, pg, db); err != nil {
			return nil, fmt.Errorf("initializing block store: %w", err)
		}
	}
	// read certificates from storage
	ucs, err := readCertificates(db, pg)
	if err != nil {
		return nil, fmt.Errorf("reading shard states from storage: %w", err)
	}
	blTree, err := NewBlockTree(db)
	if err != nil {
		return nil, fmt.Errorf("initializing block tree: %w", err)
	}
	return &BlockStore{
		hash:          hash,
		blockTree:     blTree,
		shardInfo:     ucs,
		storage:       db,
		orchestration: orchestration,
	}, nil
}

func NewFromState(hash crypto.Hash, stateMsg *abdrc.StateMsg, db keyvaluedb.KeyValueDB, orchestration Orchestration) (*BlockStore, error) {
	// Initiate store
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	certificates := make(map[partitionShard]*drctypes.ShardInfo)
	for _, siState := range stateMsg.ShardInfo {
		si := &drctypes.ShardInfo{
			Round:         siState.Round,
			Epoch:         siState.Epoch,
			RootHash:      siState.RootHash,
			PrevEpochStat: siState.PrevEpochStat,
			Stat:          siState.Stat,
			PrevEpochFees: siState.PrevEpochFees,
			Fees:          siState.Fees,
			Leader:        siState.Leader,
			LastCR: &certification.CertificationResponse{
				Partition: siState.Partition,
				Shard:     siState.Shard,
				Technical: siState.TR,
				UC:        siState.UC,
			},
		}
		pg, err := orchestration.ShardConfig(siState.Partition, siState.Shard, si.Epoch)
		if err != nil {
			return nil, fmt.Errorf("acquiring shard configuration: %w", err)
		}
		if err = si.Init(pg); err != nil {
			return nil, fmt.Errorf("init shard info (%d): %w", siState.Partition, err)
		}
		// persist changes
		if err := db.Write(certKey(siState.Partition, siState.Shard), si); err != nil {
			return nil, fmt.Errorf("failed to write shard info of partition %s into storage: %w", siState.Partition, err)
		}
		// update cache
		certificates[partitionShard{partition: siState.Partition, shard: siState.Shard.Key()}] = si
	}

	// create new root node
	rootNode, err := NewRootBlock(hash, stateMsg.CommittedHead)
	if err != nil {
		return nil, fmt.Errorf("failed to create new root node: %w", err)
	}

	blTree, err := NewBlockTreeFromRecovery(rootNode, db)
	if err != nil {
		return nil, fmt.Errorf("creating block tree from recovery: %w", err)
	}
	return &BlockStore{
		hash:          hash,
		blockTree:     blTree,
		shardInfo:     certificates,
		storage:       db,
		orchestration: orchestration,
	}, nil
}

func (x *BlockStore) ProcessTc(tc *drctypes.TimeoutCert) (rErr error) {
	if tc == nil {
		return fmt.Errorf("error tc is nil")
	}
	// persist last known TC
	if err := WriteLastTC(x.storage, tc); err != nil {
		// store DB error and continue
		rErr = fmt.Errorf("TC write failed: %w", err)
	}
	// Remove proposal/block for TC round if it exists, since quorum voted for timeout.
	// It will never be committed, hence it can be removed immediately.
	// It is fine if the block is not found, it does not matter anyway
	if err := x.blockTree.RemoveLeaf(tc.GetRound()); err != nil {
		return errors.Join(rErr, fmt.Errorf("removing timeout block %v: %w", tc.GetRound(), err))
	}
	return rErr
}

// IsChangeInProgress - return input record if sysID has a pending IR change in the pipeline or nil if no change is
// currently in the pipeline.
func (x *BlockStore) IsChangeInProgress(partition types.PartitionID, shard types.ShardID) *types.InputRecord {
	blocks := x.blockTree.GetAllUncommittedNodes()
	// go through the block we have and make sure that there is no change in progress for this partition id
	for _, b := range blocks {
		if _, ok := b.Changed[partitionShard{partition, shard.Key()}]; ok {
			return b.CurrentIR.Find(partition).IR
		}
	}
	return nil
}

func (x *BlockStore) GetDB() keyvaluedb.KeyValueDB {
	return x.storage
}

func (x *BlockStore) ProcessQc(qc *drctypes.QuorumCert) ([]*certification.CertificationResponse, error) {
	if qc == nil {
		return nil, fmt.Errorf("qc is nil")
	}
	// if we have processed it already then skip (in case we are the next leader we have already handled the QC)
	if x.GetHighQc().GetRound() >= qc.GetRound() {
		// stale qc
		return nil, nil
	}
	// add Qc to block tree
	err := x.blockTree.InsertQc(qc)
	if err != nil {
		return nil, fmt.Errorf("failed to insert QC into block tree: %w", err)
	}
	// This QC does not serve as commit QC, then we are done
	if qc.GetRound() == genesis.RootRound || qc.LedgerCommitInfo.Hash == nil {
		// NB! exception, no commit for genesis round
		return nil, nil
	}
	// If the QC commits a state
	// committed block becomes the new root and nodes to old root are removed
	committedBlock, err := x.blockTree.Commit(qc)
	if err != nil {
		return nil, err
	}
	// generate certificates for all partitions that have changes in progress
	ucs, err := committedBlock.GenerateCertificates(qc)
	if err != nil {
		return nil, fmt.Errorf("commit block failed to generate certificates for round %v: %w", committedBlock.GetRound(), err)
	}
	// update current certificates
	if err := x.updateCertificateCache(ucs); err != nil {
		return nil, fmt.Errorf("failed to update certificate cache: %w", err)
	}
	// commit blocks, the newly committed block becomes the new root in chain
	return ucs, nil
}

// Add adds new round state to pipeline and returns the new state root hash a.k.a. execStateID
func (x *BlockStore) Add(block *drctypes.BlockData, verifier IRChangeReqVerifier) ([]byte, error) {
	// verify that block for the round does not exist yet
	// if block already exists, then check that it is the same block by comparing block hash
	if b, err := x.blockTree.FindBlock(block.GetRound()); err == nil && b != nil {
		b1h := b.BlockData.Hash(crypto.SHA256)
		b2h := block.Hash(crypto.SHA256)
		// ignore if it is the same block, recovery may have added it when state was duplicated
		if bytes.Equal(b1h, b2h) {
			return b.RootHash, nil
		}
		return nil, fmt.Errorf("add block failed: different block for round %v is already in store", block.Round)
	}
	// block was not present, check parent block (QC round) is stored (if not node needs to recover)
	parentBlock, err := x.blockTree.FindBlock(block.GetParentRound())
	if err != nil {
		return nil, fmt.Errorf("add block failed: parent round %v not found, recover", block.Qc.VoteInfo.RoundNumber)
	}
	// Extend state from parent block
	exeBlock, err := NewExecutedBlock(x.hash, block, parentBlock, verifier, x.getTR)
	if err != nil {
		return nil, fmt.Errorf("error processing block round %v, %w", block.Round, err)
	}
	// append new block
	if err = x.blockTree.Add(exeBlock); err != nil {
		return nil, err
	}
	return exeBlock.RootHash, nil
}

func (x *BlockStore) getTR(partition types.PartitionID, shard types.ShardID, req *certification.BlockCertificationRequest) (certification.TechnicalRecord, error) {
	si, ok := x.shardInfo[partitionShard{partition: partition, shard: shard.Key()}]
	if !ok {
		return certification.TechnicalRecord{}, fmt.Errorf("no shard info %s - %s", partition, shard)
	}
	return si.TechnicalRecord(req, x.orchestration)
}

func (x *BlockStore) GetHighQc() *drctypes.QuorumCert {
	return x.blockTree.HighQc()
}

func (x *BlockStore) GetLastTC() (*drctypes.TimeoutCert, error) {
	return ReadLastTC(x.storage)
}

func (x *BlockStore) updateCertificateCache(certs []*certification.CertificationResponse) error {
	x.lock.Lock()
	defer x.lock.Unlock()
	for _, cr := range certs {
		siKey := partitionShard{partition: cr.Partition, shard: cr.Shard.Key()}
		si, ok := x.shardInfo[siKey]
		if !ok {
			return fmt.Errorf("shard info not found %v", siKey)
		}
		if err := si.SetLatestCert(cr); err != nil {
			return fmt.Errorf("updating certificate in ShardInfo(%v): %w", siKey, err)
		}
		if si.Epoch != cr.Technical.Epoch {
			partGene, err := x.orchestration.ShardConfig(cr.Partition, cr.Shard, cr.Technical.Epoch)
			if err != nil {
				return fmt.Errorf("reading config of the next epoch: %w", err)
			}
			si, err = si.NextEpoch(partGene)
			if err != nil {
				return fmt.Errorf("creating ShardInfo for the next epoch %v: %w", siKey, err)
			}
			x.shardInfo[siKey] = si
		}
		// persist changes
		if err := x.storage.Write(certKey(cr.Partition, cr.Shard), si); err != nil {
			// non-functional requirements? what should the root node do if it fails to persist state?
			// todo: AB-795 persistent storage failure?
			return fmt.Errorf("failed to persist shard info: %w", err)
		}
	}
	return nil
}

func (x *BlockStore) GetCertificate(id types.PartitionID, shard types.ShardID) (*certification.CertificationResponse, error) {
	x.lock.RLock()
	defer x.lock.RUnlock()
	si, f := x.shardInfo[partitionShard{partition: id, shard: shard.Key()}]
	if !f {
		return nil, fmt.Errorf("no certificate found for partition id %s", id)
	}
	return si.LastCR, nil
}

func (x *BlockStore) GetCertificates() []*types.UnicityCertificate {
	x.lock.RLock()
	defer x.lock.RUnlock()

	ucs := make([]*types.UnicityCertificate, 0, len(x.shardInfo))
	for _, v := range x.shardInfo {
		ucs = append(ucs, &v.LastCR.UC)
	}
	return ucs
}

func (x *BlockStore) ShardInfo(partition types.PartitionID, shard types.ShardID) (*drctypes.ShardInfo, error) {
	if si, ok := x.shardInfo[partitionShard{partition: partition, shard: shard.Key()}]; ok {
		return si, nil
	}
	return nil, fmt.Errorf("no shard info found for {%s : %s}", partition, shard)
}

func (x *BlockStore) GetState() *abdrc.StateMsg {
	committedBlock := x.blockTree.Root()
	pendingBlocks := x.blockTree.GetAllUncommittedNodes()
	pending := make([]*drctypes.BlockData, len(pendingBlocks))
	for i, b := range pendingBlocks {
		pending[i] = b.BlockData
	}
	// sort blocks by round before sending
	slices.SortFunc(pending, func(a, b *drctypes.BlockData) int {
		return cmp.Compare(a.GetRound(), b.GetRound())
	})

	return &abdrc.StateMsg{
		ShardInfo: toRecoveryShardInfo(x.shardInfo),
		CommittedHead: &abdrc.CommittedBlock{
			Block:    committedBlock.BlockData,
			Ir:       ToRecoveryInputData(committedBlock.CurrentIR),
			Qc:       committedBlock.Qc,
			CommitQc: committedBlock.CommitQc,
		},
		BlockData: pending,
	}
}

/*
Block returns block for given round.
When store doesn't have block for the round it returns error.
*/
func (x *BlockStore) Block(round uint64) (*ExecutedBlock, error) {
	return x.blockTree.FindBlock(round)
}

// StoreLastVote stores last sent vote message by this node
func (x *BlockStore) StoreLastVote(vote any) error {
	return WriteVote(x.storage, vote)
}

// ReadLastVote returns last sent vote message by this node
func (x *BlockStore) ReadLastVote() (any, error) {
	return ReadVote(x.storage)
}

func toRecoveryShardInfo(data map[partitionShard]*drctypes.ShardInfo) []abdrc.ShardInfo {
	si := make([]abdrc.ShardInfo, len(data))
	idx := 0
	for k, v := range data {
		si[idx].Partition = k.partition
		si[idx].Shard = v.LastCR.Shard
		si[idx].Round = v.Round
		si[idx].Epoch = v.Epoch
		si[idx].RootHash = v.RootHash
		si[idx].PrevEpochStat = v.PrevEpochStat
		si[idx].Stat = v.Stat
		si[idx].PrevEpochFees = v.PrevEpochFees
		si[idx].Fees = v.Fees
		si[idx].Leader = v.Leader
		si[idx].UC = v.LastCR.UC
		si[idx].TR = v.LastCR.Technical
		idx++
	}
	return si
}

// ToRecoveryInputData function for type conversion
func ToRecoveryInputData(data []*InputData) []*abdrc.InputData {
	inputData := make([]*abdrc.InputData, len(data))
	for i, d := range data {
		inputData[i] = &abdrc.InputData{
			Partition: d.Partition,
			Shard:     d.Shard,
			Ir:        d.IR,
			Technical: d.Technical,
			PDRHash:   d.PDRHash,
		}
	}
	return inputData
}
