package storage

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"maps"
	"strings"
	"sync"

	"github.com/alphabill-org/alphabill/keyvaluedb"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/types"
)

type (
	BlockStore struct {
		hash         gocrypto.Hash // hash algorithm
		blockTree    *BlockTree
		storage      keyvaluedb.KeyValueDB
		certificates map[types.SystemID32]*types.UnicityCertificate // cashed
		lock         sync.RWMutex
	}
)

func UnicityCertificatesFromGenesis(pg []*genesis.GenesisPartitionRecord) map[types.SystemID32]*types.UnicityCertificate {
	var certs = make(map[types.SystemID32]*types.UnicityCertificate)
	for _, partition := range pg {
		identifier, _ := partition.GetSystemDescriptionRecord().GetSystemIdentifier().Id32()
		certs[identifier] = partition.Certificate
	}
	return certs
}

func storeGenesisInit(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) error {
	// nil is returned if no value is in DB
	genesisBlock := NewGenesisBlock(hash, pg)
	ucs := UnicityCertificatesFromGenesis(pg)
	for id, cert := range ucs {
		if err := db.Write(certKey(id.ToSystemID()), cert); err != nil {
			return fmt.Errorf("certificate %X write failed, %w", id, err)
		}
	}
	if err := blockStoreGenesisInit(genesisBlock, db); err != nil {
		return fmt.Errorf("block store genesis init failed, %w", err)
	}
	return nil
}

func readCertificates(db keyvaluedb.KeyValueDB) (ucs map[types.SystemID32]*types.UnicityCertificate, err error) {
	// read certificates from storage
	itr := db.Find([]byte(certPrefix))
	defer func() { err = errors.Join(err, itr.Close()) }()
	ucs = make(map[types.SystemID32]*types.UnicityCertificate)
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), certPrefix); itr.Next() {
		var uc types.UnicityCertificate
		if err = itr.Value(&uc); err != nil {
			return nil, fmt.Errorf("certificate read error, %w", err)
		}
		id, _ := uc.UnicityTreeCertificate.SystemIdentifier.Id32()
		ucs[id] = &uc
	}
	return ucs, err
}

func New(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) (block *BlockStore, err error) {
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
		return nil, fmt.Errorf("failed to read block store, %w", err)
	}
	if empty {
		if err = storeGenesisInit(hash, pg, db); err != nil {
			return nil, fmt.Errorf("block store init failed, %w", err)
		}
	}
	// read certificates from storage
	ucs, err := readCertificates(db)
	if err != nil {
		return nil, fmt.Errorf("init failed, %w", err)
	}
	blTree, err := NewBlockTree(db)
	if err != nil {
		return nil, fmt.Errorf("init failed, %w", err)
	}
	return &BlockStore{
		hash:         hash,
		blockTree:    blTree,
		certificates: ucs,
		storage:      db,
	}, nil
}

func NewFromState(hash gocrypto.Hash, rRootBlock *abdrc.RecoveryBlock, certs []*types.UnicityCertificate, db keyvaluedb.KeyValueDB) (*BlockStore, error) {
	// Initiate store
	if db == nil {
		return nil, errors.New("storage is nil")
	}
	certificates := make(map[types.SystemID32]*types.UnicityCertificate)
	for _, cert := range certs {
		id, err := cert.UnicityTreeCertificate.SystemIdentifier.Id32()
		if err != nil {
			return nil, fmt.Errorf("certificate has invalid partition id %X: %w", cert.UnicityTreeCertificate.SystemIdentifier, err)
		}
		// persist changes
		if err = db.Write(certKey(cert.UnicityTreeCertificate.SystemIdentifier), cert); err != nil {
			return nil, fmt.Errorf("failed to write certificate of partition %s into storage: %w", id, err)
		}
		// update cache
		certificates[id] = cert
	}
	// create new root node

	rootNode, err := NewRootBlockFromRecovery(hash, rRootBlock)
	if err != nil {
		return nil, fmt.Errorf("failed to create new root node: %w", err)
	}

	blTree, err := NewBlockTreeFromRecovery(rootNode, nil, db)
	if err != nil {
		return nil, fmt.Errorf("creating block tree from recovery: %w", err)
	}
	return &BlockStore{
		hash:         hash,
		blockTree:    blTree,
		certificates: certificates,
		storage:      db,
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

func (x *BlockStore) IsChangeInProgress(sysId types.SystemID32) bool {
	blocks := x.blockTree.GetAllUncommittedNodes()
	// go through the block we have and make sure that there is no change in progress for this system id
	for _, b := range blocks {
		if b.Changed.Contains(sysId) {
			return true
		}
	}
	return false
}

func (x *BlockStore) GetBlockRootHash(round uint64) ([]byte, error) {
	b, err := x.blockTree.FindBlock(round)
	if err != nil {
		return nil, fmt.Errorf("get block root hash failed, %w", err)
	}
	return b.RootHash, nil
}

func (x *BlockStore) GetDB() keyvaluedb.KeyValueDB {
	return x.storage
}

func (x *BlockStore) ProcessQc(qc *drctypes.QuorumCert) (map[types.SystemID32]*types.UnicityCertificate, error) {
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
	b, err := x.blockTree.FindBlock(block.GetRound())
	if err == nil {
		b1h := b.BlockData.Hash(gocrypto.SHA256)
		b2h := block.Hash(gocrypto.SHA256)
		// block was found, ignore if it is the same block, recovery may have added it when state was duplicated
		if bytes.Equal(b1h, b2h) {
			return b.RootHash, nil
		}
		return nil, fmt.Errorf("add block failed: different block for round %v is already in store", block.Round)
	}
	// QC round is parent block round
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

func (x *BlockStore) updateCertificateCache(certs map[types.SystemID32]*types.UnicityCertificate) error {
	x.lock.Lock()
	defer x.lock.Unlock()
	for id, uc := range certs {
		// persist changes
		if err := x.storage.Write(certKey(id.ToSystemID()), uc); err != nil {
			// non-functional requirements? what should the root node do if it fails to persist state?
			// todo: AB-795 persistent storage failure?
			return fmt.Errorf("failed to write certificate into storage: %w", err)
		}
		// update cache
		x.certificates[id] = uc
	}
	return nil
}

func (x *BlockStore) GetCertificate(id types.SystemID32) (*types.UnicityCertificate, error) {
	x.lock.RLock()
	defer x.lock.RUnlock()
	uc, f := x.certificates[id]
	if !f {
		return nil, fmt.Errorf("no certificate found for system id %s", id)
	}
	return uc, nil
}

func (x *BlockStore) GetCertificates() map[types.SystemID32]*types.UnicityCertificate {
	x.lock.RLock()
	defer x.lock.RUnlock()
	return maps.Clone(x.certificates)
}

func (x *BlockStore) GetRoot() *ExecutedBlock {
	return x.blockTree.Root()
}

func ToRecoveryInputData(data []*InputData) []*abdrc.InputData {
	inputData := make([]*abdrc.InputData, len(data))
	for i, d := range data {
		inputData[i] = &abdrc.InputData{
			SysID: d.SysID,
			Ir:    d.IR,
			Sdrh:  d.Sdrh,
		}
	}
	return inputData
}

func (x *BlockStore) GetPendingBlocks() []*ExecutedBlock {
	return x.blockTree.GetAllUncommittedNodes()
}

/*
Block returns block for given round.
When store doesn't have block for the round it returns error.
*/
func (x *BlockStore) Block(round uint64) (*drctypes.BlockData, error) {
	eb, err := x.blockTree.FindBlock(round)
	if err != nil {
		return nil, err
	}
	return eb.BlockData, nil
}

// StoreLastVote - store last sent vote message
func (x *BlockStore) StoreLastVote(vote any) error {
	return WriteVote(x.storage, vote)
}

// ReadLastVote - read last vote message
func (x *BlockStore) ReadLastVote() (any, error) {
	return ReadVote(x.storage)
}
