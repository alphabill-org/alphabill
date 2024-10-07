package storage

import (
	"bytes"
	"cmp"
	gocrypto "crypto"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
)

type (
	ConfigurationStore interface {
		GetConfiguration(round uint64) (*genesis.RootGenesis, uint64, error)
	}

	BlockStore struct {
		mu           sync.RWMutex
		hash         gocrypto.Hash // hash algorithm
		blockTree    *BlockTree
		storage      keyvaluedb.KeyValueDB
		cfgStore     ConfigurationStore
		cfgVersion   atomic.Uint64
		currentRound func() uint64
		certificates map[partitionShard]*certification.CertificationResponse // cache
	}

	partitionShard struct {
		partition types.SystemID
		shard     string // types.ShardID is not comparable
	}
)

func certificatesFromGenesis(pg []*genesis.GenesisPartitionRecord) map[partitionShard]*certification.CertificationResponse {
	var crs = make(map[partitionShard]*certification.CertificationResponse)
	for _, partition := range pg {
		// TODO: generate per shard! Genesis must support shards...
		cr := &certification.CertificationResponse{
			Partition: partition.GetSystemDescriptionRecord().GetSystemIdentifier(),
			Shard:     types.ShardID{},
			Technical: certification.TechnicalRecord{},
			UC:        *partition.Certificate,
		}
		ps := partitionShard{partition: cr.Partition, shard: cr.Shard.Key()}
		crs[ps] = cr
	}
	return crs
}

func storeGenesisInit(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) error {
	// nil is returned if no value is in DB
	genesisBlock := NewGenesisBlock(hash, pg)
	if err := blockStoreGenesisInit(genesisBlock, db); err != nil {
		return fmt.Errorf("block store genesis init failed, %w", err)
	}
	return nil
}

func readCertificates(db keyvaluedb.KeyValueDB) (ucs map[partitionShard]*certification.CertificationResponse, err error) {
	// read certificates from storage
	itr := db.Find([]byte(certPrefix))
	defer func() { err = errors.Join(err, itr.Close()) }()
	ucs = make(map[partitionShard]*certification.CertificationResponse)
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), certPrefix); itr.Next() {
		var uc certification.CertificationResponse
		if err = itr.Value(&uc); err != nil {
			return nil, fmt.Errorf("reading certificate from storage: %w", err)
		}
		ucs[partitionShard{partition: uc.Partition, shard: uc.Shard.Key()}] = &uc
	}
	return ucs, err
}

func New(hash gocrypto.Hash, cfgStore ConfigurationStore, db keyvaluedb.KeyValueDB, currentRound func() uint64) (block *BlockStore, err error) {
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	if cfgStore == nil {
		return nil, errors.New("configuration store is nil")
	}
	if currentRound == nil {
		return nil, errors.New("current round provider is nil")
	}

	// First start, initiate from genesis data
	empty, err := keyvaluedb.IsEmpty(db)
	if err != nil {
		return nil, fmt.Errorf("failed to read block store, %w", err)
	}
	if empty {
		genesisCfg, _, err := cfgStore.GetConfiguration(genesis.RootRound)
		if err != nil {
			return nil, fmt.Errorf("loading genesis configuration failed, %w", err)
		}
		if err = storeGenesisInit(hash, genesisCfg.Partitions, db); err != nil {
			return nil, fmt.Errorf("block store init failed, %w", err)
		}
	}
	// read certificates from storage, empty for genesis
	ucs, err := readCertificates(db)
	if err != nil {
		return nil, fmt.Errorf("init failed, %w", err)
	}
	blTree, err := NewBlockTree(db)
	if err != nil {
		return nil, fmt.Errorf("init failed, %w", err)
	}
	// cfgVersion is set to 0, which means certificates will be
	// updated according to cfg with the next call to GetCertificate(s)
	return &BlockStore{
		hash:         hash,
		blockTree:    blTree,
		storage:      db,
		cfgStore:     cfgStore,
		currentRound: currentRound,
		certificates: ucs,
	}, nil
}

func NewFromState(hash gocrypto.Hash, cfgStore ConfigurationStore, db keyvaluedb.KeyValueDB, currentRound func() uint64, stateMsg *abdrc.StateMsg) (*BlockStore, error) {
	// Initiate store
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	if cfgStore == nil {
		return nil, errors.New("configuration store is nil")
	}
	if currentRound == nil {
		return nil, errors.New("current round provider is nil")
	}

	certificates := make(map[partitionShard]*certification.CertificationResponse)
	for _, cert := range stateMsg.Certificates {
		id := cert.UC.UnicityTreeCertificate.SystemIdentifier
		// persist changes
		if err := db.Write(certKey(id, cert.Shard), cert); err != nil {
			return nil, fmt.Errorf("failed to write certificate of partition %s into storage: %w", id, err)
		}
		// update cache
		certificates[partitionShard{partition: id, shard: cert.Shard.Key()}] = cert
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
	// cfgVersion is set to 0, which means certificates will be
	// updated according to cfg with the next call to GetCertificate(s)
	return &BlockStore{
		hash:         hash,
		blockTree:    blTree,
		storage:      db,
		cfgStore:     cfgStore,
		currentRound: currentRound,
		certificates: certificates,
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
func (x *BlockStore) IsChangeInProgress(sysId types.SystemID) *types.InputRecord {
	blocks := x.blockTree.GetAllUncommittedNodes()
	// go through the block we have and make sure that there is no change in progress for this system id
	for _, b := range blocks {
		if slices.Contains(b.Changed, sysId) {
			return b.CurrentIR.Find(sysId).IR
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
		b1h := b.BlockData.Hash(gocrypto.SHA256)
		b2h := block.Hash(gocrypto.SHA256)
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
	exeBlock, err := NewExecutedBlock(x.hash, block, parentBlock, verifier)
	if err != nil {
		return nil, fmt.Errorf("error processing block round %v, %w", block.Round, err)
	}
	// append new block
	if err = x.blockTree.Add(exeBlock); err != nil {
		return nil, err
	}
	return exeBlock.RootHash, nil
}

func (x *BlockStore) GetHighQc() *drctypes.QuorumCert {
	return x.blockTree.HighQc()
}

func (x *BlockStore) GetLastTC() (*drctypes.TimeoutCert, error) {
	return ReadLastTC(x.storage)
}

func (x *BlockStore) updateCertificateCache(certs []*certification.CertificationResponse) error {
	x.mu.Lock()
	defer x.mu.Unlock()
	for _, uc := range certs {
		// persist changes
		if err := x.storage.Write(certKey(uc.Partition, uc.Shard), uc); err != nil {
			// non-functional requirements? what should the root node do if it fails to persist state?
			// todo: AB-795 persistent storage failure?
			return fmt.Errorf("failed to write certificate into storage: %w", err)
		}
		// update cache
		x.certificates[partitionShard{partition: uc.Partition, shard: uc.Shard.Key()}] = uc
	}
	return nil
}

func (x *BlockStore) GetCertificate(id types.SystemID, shard types.ShardID) (*certification.CertificationResponse, error) {
	if err := x.loadConfig(); err != nil {
		return nil, fmt.Errorf("loading new configuration: %w", err)
	}

	x.mu.RLock()
	defer x.mu.RUnlock()
	uc, f := x.certificates[partitionShard{partition: id, shard: shard.Key()}]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %s", id)
	}
	return uc, nil
}

func (x *BlockStore) GetCertificates() ([]*certification.CertificationResponse, error) {
	if err := x.loadConfig(); err != nil {
		return nil, fmt.Errorf("loading new configuration: %w", err)
	}

	x.mu.RLock()
	defer x.mu.RUnlock()
	return slices.Collect(maps.Values(x.certificates)), nil
}

func (x *BlockStore) GetState() (*abdrc.StateMsg, error) {
	certs, err := x.GetCertificates()
	if err != nil {
		return nil, fmt.Errorf("loading certificates: %w", err)
	}
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
		Certificates: certs,
		CommittedHead: &abdrc.CommittedBlock{
			Block:    committedBlock.BlockData,
			Ir:       ToRecoveryInputData(committedBlock.CurrentIR),
			Qc:       committedBlock.Qc,
			CommitQc: committedBlock.CommitQc,
		},
		BlockData: pending,
	}, nil
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

/*
loadConfig loads the root chain configuration for the current round and
updates the list of Unicity Certificates with those of added and removed partitions.
*/
func (x *BlockStore) loadConfig() error {
	round := x.currentRound()
	cfg, version, err := x.cfgStore.GetConfiguration(round)
	if err != nil {
		return fmt.Errorf("loading root chain configuration: %w", err)
	}
	if x.cfgVersion.Load() >= version {
		// Latest configuration already loaded
		return nil
	}

	x.mu.Lock()
	defer x.mu.Unlock()

	// double-check to see if someone else already loaded it while we waited on lock
	if x.cfgVersion.Load() >= version {
		return nil
	}

	newCrs := certificatesFromGenesis(cfg.Partitions)

	// find deleted partitions
	for ps, cr := range x.certificates {
		if _, ok := newCrs[ps]; !ok {
			if err := x.storage.Delete(certKey(cr.Partition, cr.Shard)); err != nil {
				return fmt.Errorf("failed to delete certificate from storage: %w", err)
			}
			delete(x.certificates, ps)
		}
	}
	// find added partitions
	for ps, newCr := range newCrs {
		if _, ok := x.certificates[ps]; !ok {
			if err := x.storage.Write(certKey(newCr.Partition, newCr.Shard), newCr); err != nil {
				return fmt.Errorf("failed to write certificate into storage: %w", err)
			}
			x.certificates[ps] = newCr
		}
	}

	x.cfgVersion.Store(version)
	return nil
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
			Sdrh:      d.PDRHash,
		}
	}
	return inputData
}
