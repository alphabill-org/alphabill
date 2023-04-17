package storage

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/keyvaluedb"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/ab_consensus"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type (
	BlockStore struct {
		hash         gocrypto.Hash // hash algorithm
		blockTree    *BlockTree
		storage      keyvaluedb.KeyValueDB
		Certificates map[protocol.SystemIdentifier]*certificates.UnicityCertificate // cashed
		lock         sync.RWMutex
	}
)

func UnicityCertificatesFromGenesis(pg []*genesis.GenesisPartitionRecord) map[protocol.SystemIdentifier]*certificates.UnicityCertificate {
	var certs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, partition := range pg {
		identifier := partition.GetSystemIdentifierString()
		certs[identifier] = partition.Certificate
	}
	return certs
}

func storeGenesisInit(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) error {
	// nil is returned if no value is in DB
	genesisBlock := NewExecutedBlockFromGenesis(hash, pg)
	ucs := UnicityCertificatesFromGenesis(pg)
	for id, cert := range ucs {
		if err := db.Write(certKey(id.Bytes()), cert); err != nil {
			return fmt.Errorf("certificate %X write failed, %w", id, err)
		}
	}
	if err := blockStoreGenesisInit(genesisBlock, db); err != nil {
		return fmt.Errorf("block store genesis init failed, %w", err)
	}
	return nil
}

func readCertificates(db keyvaluedb.KeyValueDB) (ucs map[protocol.SystemIdentifier]*certificates.UnicityCertificate, err error) {
	// read certificates from storage
	itr := db.Find([]byte(certPrefix))
	defer func() { err = errors.Join(err, itr.Close()) }()
	ucs = make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for ; itr.Valid() && strings.HasPrefix(string(itr.Key()), certPrefix); itr.Next() {
		var uc certificates.UnicityCertificate
		if err = itr.Value(&uc); err != nil {
			return nil, fmt.Errorf("certificate read error, %w", err)
		}
		id := uc.UnicityTreeCertificate.SystemIdentifier
		ucs[protocol.SystemIdentifier(id)] = &uc
	}
	return ucs, err
}

func NewBlockStore(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, db keyvaluedb.KeyValueDB) (block *BlockStore, err error) {
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
		Certificates: ucs,
		storage:      db,
	}, nil
}

func (x *BlockStore) ProcessTc(tc *ab_consensus.TimeoutCert) error {
	if tc == nil {
		return fmt.Errorf("error tc is nil")
	}
	// Remove proposal for TC round if it exists, since quorum voted for timeout, it will never be committed
	// So we will prune the block now, also it is ok, if we do not have the block, it does not matter anyway
	if err := x.blockTree.RemoveLeaf(tc.Timeout.Round); err != nil {
		logger.Warning("Unexpected error when removing timeout block %v, %v", tc.Timeout.Round, err)
	}
	return nil
}

func (x *BlockStore) IsChangeInProgress(sysId protocol.SystemIdentifier) bool {
	blocks := x.blockTree.GetAllUncommittedNodes()
	// go through the block we have and make sure that there is no change in progress for this system id
	for _, b := range blocks {
		if b.Changed.Find(sysId.Bytes()) == true {
			return true
		}
	}
	return false
}

func (x *BlockStore) GetBlockRootHash(round uint64) ([]byte, error) {
	b, err := x.blockTree.FindBlock(round)
	if err != nil {
		return nil, err
	}
	return b.RootHash, nil
}

func (x *BlockStore) ProcessQc(qc *ab_consensus.QuorumCert) (map[protocol.SystemIdentifier]*certificates.UnicityCertificate, error) {
	if qc == nil {
		return nil, fmt.Errorf("qc is nil")
	}
	// if we have processed it already then skip (in case we are the next leader we have already handled the QC)
	if x.GetHighQc().VoteInfo.RoundNumber >= qc.VoteInfo.RoundNumber {
		// stale qc
		return nil, nil
	}
	// add Qc to block tree
	err := x.blockTree.InsertQc(qc, x.storage)
	if err != nil {
		return nil, fmt.Errorf("block store qc handling failed, %w", err)
	}
	// This QC does not serve as commit QC, then we are done
	if qc.VoteInfo.RoundNumber == genesis.RootRound || qc.LedgerCommitInfo.RootHash == nil {
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
		return nil, fmt.Errorf("commit block %v error, %w", committedBlock.BlockData.Round, err)
	}
	// update current certificates
	x.updateCertificateCache(ucs)
	// commit blocks, the newly committed block becomes the new root in chain
	return ucs, nil
}

// Add adds new round state to pipeline and returns the new state root hash a.k.a. execStateID
func (x *BlockStore) Add(block *ab_consensus.BlockData, verifier IRChangeReqVerifier) ([]byte, error) {
	// verify that block for the round does not exist yet
	_, err := x.blockTree.FindBlock(block.Round)
	if err == nil {
		return nil, fmt.Errorf("add block failed: block for round %v already in store", block.Round)
	}
	parentBlock, err := x.blockTree.FindBlock(block.Qc.VoteInfo.RoundNumber)
	if err != nil {
		return nil, fmt.Errorf("add block failed: parent round %v not found, recover", block.Qc.VoteInfo.RoundNumber)
	}
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

func (x *BlockStore) GetHighQc() *ab_consensus.QuorumCert {
	return x.blockTree.HighQc()
}

func (x *BlockStore) updateCertificateCache(certs map[protocol.SystemIdentifier]*certificates.UnicityCertificate) error {
	x.lock.Lock()
	defer x.lock.Unlock()
	for id, uc := range certs {
		x.Certificates[id] = uc
		// and persist changes
		// todo: AB-795 persistent storage failure?
		if err := x.storage.Write(certKey(id.Bytes()), uc); err != nil {
			return fmt.Errorf("failed to update certificates, %w", err)
		}
	}
	return nil
}

func (x *BlockStore) GetCertificates() map[protocol.SystemIdentifier]*certificates.UnicityCertificate {
	x.lock.RLock()
	defer x.lock.RUnlock()
	return x.Certificates
}

func (x *BlockStore) GetRoot() *ExecutedBlock {
	return x.blockTree.Root()
}

func (x *BlockStore) UpdateCertificates(cert []*certificates.UnicityCertificate) {
	newerCerts := make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for _, c := range cert {
		id := protocol.SystemIdentifier(c.UnicityTreeCertificate.SystemIdentifier)
		cachedCert, found := x.Certificates[id]
		if !found || cachedCert.UnicitySeal.RootRoundInfo.RoundNumber < c.UnicitySeal.RootRoundInfo.RoundNumber {
			newerCerts[id] = c
		}
	}
	x.updateCertificateCache(newerCerts)
}

func ToRecoveryInputData(data []*InputData) []*ab_consensus.InputData {
	inputData := make([]*ab_consensus.InputData, len(data))
	for i, d := range data {
		inputData[i] = &ab_consensus.InputData{
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

func (x *BlockStore) RecoverState(rRootBlock *ab_consensus.RecoveryBlock, rNodes []*ab_consensus.RecoveryBlock, verifier IRChangeReqVerifier) error {
	rootNode, err := NewExecutedBlockFromRecovery(x.hash, rRootBlock, verifier)
	if err != nil {
		return fmt.Errorf("state recovery failed, %w", err)
	}
	nodes := make([]*ExecutedBlock, len(rNodes))
	for i, n := range rNodes {
		var executedBlock *ExecutedBlock
		executedBlock, err = NewExecutedBlockFromRecovery(x.hash, n, verifier)
		if err != nil {
			return fmt.Errorf("state recovery failed, %w", err)
		}
		nodes[i] = executedBlock
	}
	bt, err := NewBlockTreeFromRecovery(rootNode, nodes, x.storage)
	if err != nil {
		return fmt.Errorf("state recovery, failed to create block tree from recovered blocks, %w", err)
	}
	// replace block tree
	x.blockTree = bt
	return nil
}
