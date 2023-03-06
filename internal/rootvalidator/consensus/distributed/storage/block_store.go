package storage

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
)

type (
	BlockStore struct {
		hash         gocrypto.Hash // hash algorithm
		blockTree    *BlockTree
		storage      *Storage
		Certificates map[protocol.SystemIdentifier]*certificates.UnicityCertificate // cashed
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

func storeGenesisInit(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, s *Storage) error {
	// nil is returned if no value is in DB
	genesisBlock := NewExecutedBlockFromGenesis(hash, pg)
	ucs := UnicityCertificatesFromGenesis(pg)
	certStore := s.GetCertificatesDB()
	for id, cert := range ucs {
		if err := certStore.Write(id.Bytes(), cert); err != nil {
			return fmt.Errorf("certificates store genesis init failed, %w", err)
		}
	}
	if err := blockStoreGenesisInit(genesisBlock, s.GetBlocksDB()); err != nil {
		return fmt.Errorf("block store genesis init failed, %w", err)
	}
	return nil
}

func NewBlockStore(hash gocrypto.Hash, pg []*genesis.GenesisPartitionRecord, s *Storage) (*BlockStore, error) {
	// Initiate store
	if pg == nil {
		return nil, errors.New("genesis record is nil")
	}
	if s == nil {
		return nil, errors.New("storage is nil")
	}
	// First start, initiate from genesis data
	if s.blocksDB.Empty() {
		if err := storeGenesisInit(hash, pg, s); err != nil {
			return nil, fmt.Errorf("block store init failed, %w", err)
		}
	}
	certStore := s.GetCertificatesDB()
	// read certificates from storage
	itr := certStore.First()
	defer itr.Close()
	ucs := make(map[protocol.SystemIdentifier]*certificates.UnicityCertificate)
	for ; itr.Valid(); itr.Next() {
		id := protocol.SystemIdentifier(itr.Key())
		var uc certificates.UnicityCertificate
		if err := itr.Value(&uc); err != nil {
			return nil, fmt.Errorf("read certificated from db failed, %w", err)
		}
		ucs[id] = &uc
	}
	blTree, err := NewBlockTree(s.GetBlocksDB())
	if err != nil {
		return nil, fmt.Errorf("block tree init failed, %w", err)
	}
	return &BlockStore{
		hash:         hash,
		blockTree:    blTree,
		Certificates: ucs,
		storage:      s,
	}, nil
}

func (x *BlockStore) ProcessTc(tc *atomic_broadcast.TimeoutCert) error {
	if tc == nil {
		return fmt.Errorf("error tc is nil")
	}
	// Remove proposal for TC round if it exists, since quorum voted for timeout, it will never be committed
	// So we will prune the block now, also it is ok, if we do not have the block, it does not matter anyway
	_ = x.blockTree.RemoveLeaf(tc.Timeout.Round)
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

func (x *BlockStore) ProcessQc(qc *atomic_broadcast.QuorumCert) (map[protocol.SystemIdentifier]*certificates.UnicityCertificate, error) {
	if qc == nil {
		return nil, fmt.Errorf("qc is nil")
	}
	// if we have processed it already then skip (in case we are the next leader we have already handled the QC)
	if x.GetHighQc().VoteInfo.RoundNumber >= qc.VoteInfo.RoundNumber {
		// stale qc
		return nil, nil
	}
	// add Qc to block tree
	err := x.blockTree.InsertQc(qc, x.storage.GetBlocksDB())
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
	for id, uc := range ucs {
		x.Certificates[id] = uc
		// persist changes
		x.storage.GetCertificatesDB().Write(id.Bytes(), uc)
	}
	// commit blocks, the newly committed block becomes the new root in chain
	return ucs, nil
}

// Add adds new round state to pipeline and returns the new state root hash a.k.a. execStateID
func (x *BlockStore) Add(block *atomic_broadcast.BlockData, verifier IRChangeReqVerifier) ([]byte, error) {
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

func (x *BlockStore) GetHighQc() *atomic_broadcast.QuorumCert {
	return x.blockTree.HighQc()
}

func (x *BlockStore) GetCertificates() map[protocol.SystemIdentifier]*certificates.UnicityCertificate {
	return x.Certificates
}

func (x *BlockStore) GetRoot() *ExecutedBlock {
	return x.blockTree.Root()
}
