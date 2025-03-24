package root

import (
	"crypto"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"

	"github.com/alphabill-org/alphabill/keyvaluedb/memorydb"
	// "github.com/alphabill-org/alphabill/rootchain/consensus/storage"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
	"github.com/stretchr/testify/require"
)

func StoredBlockWithActivatedShard(t *testing.T, shardConf *types.PartitionDescriptionRecord, signers map[string]abcrypto.Signer) *memorydb.MemoryDB {
	db, err := memorydb.New()
	require.NoError(t, err)
	genesisBlock := newTestGenesisBlock(t, shardConf, signers)
	require.NoError(t, storage.WriteBlock(db, genesisBlock))
	return db
}

func newTestGenesisBlock(t *testing.T, shardConf *types.PartitionDescriptionRecord, signers map[string]abcrypto.Signer) *storage.ExecutedBlock {
	hashAlgo := crypto.SHA256
	genesisBlock := &drctypes.BlockData{
		Version:   1,
		Author:    "testgenesis",
		Round:     drctypes.GenesisRootRound,
		Epoch:     drctypes.GenesisRootEpoch,
		Timestamp: types.GenesisTime,
		Payload:   nil, // no IR change requests
		Qc:        nil, // no parent QC
	}

	si, err := storage.NewShardInfo(shardConf, hashAlgo)
	require.NoError(t, err)

	ir, err := storage.NewShardInputData(si, hashAlgo)
	require.NoError(t, err)

	irs := storage.InputRecords{ir}
	ut, err := irs.UnicityTree(hashAlgo)
	require.NoError(t, err)

	psID := types.PartitionShardID{PartitionID: si.PartitionID, ShardID: si.ShardID.Key()}
	commitQc := createCommitQc(t, genesisBlock, ut.RootHash(), hashAlgo, signers)
	executedBlock := &storage.ExecutedBlock{
		BlockData: genesisBlock,
		HashAlgo:  hashAlgo,
		CurrentIR: irs,
		Changed:   map[types.PartitionShardID]struct{}{psID: struct{}{}},
		ShardInfo: map[types.PartitionShardID]*storage.ShardInfo{psID: si},

		// the same QC accepts the genesis block and commits it, usually commit comes later
		Qc:        commitQc,
		CommitQc:  commitQc,
		RootHash:  commitQc.LedgerCommitInfo.Hash,
	}

	crs, err := executedBlock.GenerateCertificates(commitQc)
	require.NoError(t, err)
	require.Len(t, crs, 1)
	require.NotNil(t, si.LastCR)
	require.NotNil(t, si.LastCR.UC)

	// Changed set was necessary to generate certificates with GenerateCertificates,
	// set it to nil so that certificates won't be generated again when CM is run
	executedBlock.Changed = nil
	return executedBlock
}

func createCommitQc(t *testing.T, genesisBlock *drctypes.BlockData, rootHash []byte, hashAlgo crypto.Hash, signers map[string]abcrypto.Signer) *drctypes.QuorumCert {
	// Info about the round that commits the genesis block.
	// GenesisRootRound "produced" the genesis block and also commits it.
	commitRoundInfo := &drctypes.RoundInfo{
		Version:           1,
		RoundNumber:       genesisBlock.Round,
		Epoch:             genesisBlock.Epoch,
		Timestamp:         genesisBlock.Timestamp,
		ParentRoundNumber: 0, // no parent block
		CurrentRootHash:   rootHash,
	}
	commitRoundInfoHash, err := commitRoundInfo.Hash(hashAlgo)
	require.NoError(t, err)

	// QC that commits the genesis block
	commitQc := &drctypes.QuorumCert{
		VoteInfo: commitRoundInfo,
		LedgerCommitInfo: &types.UnicitySeal{
			Version:              1,
			NetworkID:            5,
			// Usually the round that gets committed is different from
			// the round that commits, but for genesis block they are the same.
			RootChainRoundNumber: commitRoundInfo.RoundNumber,
			Epoch:                commitRoundInfo.Epoch,
			Timestamp:            commitRoundInfo.Timestamp,
			Hash:                 commitRoundInfo.CurrentRootHash,
			PreviousHash:         commitRoundInfoHash,
		},
	}

	for nodeID, signer := range signers {
		require.NoError(t, commitQc.LedgerCommitInfo.Sign(nodeID, signer))
	}
	commitQc.Signatures = commitQc.LedgerCommitInfo.Signatures
	return commitQc
}
