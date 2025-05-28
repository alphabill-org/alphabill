package storage

import (
	"crypto"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

func Test_BoltDB_Block(t *testing.T) {
	tempDir := t.TempDir()

	blockForRound := func(round uint64) *ExecutedBlock {
		return &ExecutedBlock{
			BlockData: &rctypes.BlockData{
				Version: 1,
				Author:  "test",
				Round:   round,
				Payload: &rctypes.Payload{},
				Qc: &rctypes.QuorumCert{
					VoteInfo: &rctypes.RoundInfo{RoundNumber: round - 1, ParentRoundNumber: round - 2},
					LedgerCommitInfo: &types.UnicitySeal{
						Version:              1,
						RootChainRoundNumber: round - 1,
						PreviousHash:         test.RandomBytes(32),
						Hash:                 test.RandomBytes(32),
					},
				},
			},
			HashAlgo:   crypto.SHA256,
			RootHash:   test.RandomBytes(32),
			Qc:         &rctypes.QuorumCert{},
			CommitQc:   nil,
			ShardState: ShardStates{},
		}
	}

	t.Run("invalid database", func(t *testing.T) {
		// no blocks bucket
		db, err := NewBoltStorage(filepath.Join(tempDir, "invalid.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		err = db.db.Update(func(tx *bbolt.Tx) error {
			return tx.DeleteBucket(bucketBlocks)
		})
		require.NoError(t, err)

		b := ExecutedBlock{BlockData: &rctypes.BlockData{Round: 4}}
		require.ErrorIs(t, db.WriteBlock(&b, false), errNoBlocksBucket)

		blocks, err := db.LoadBlocks()
		require.ErrorIs(t, err, errNoBlocksBucket)
		require.Empty(t, blocks)
	})

	t.Run("save - load", func(t *testing.T) {
		// verify that we get back the same data we stored
		db, err := NewBoltStorage(filepath.Join(tempDir, "save_load.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		pdr := newShardConf(t)
		b := genesisBlockWithShard(t, pdr)
		require.NoError(t, err)
		require.NoError(t, db.WriteBlock(b, false))

		blocks, err := db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 1) {
			// set the fields which are not persisted to zero values
			key := types.PartitionShardID{PartitionID: pdr.PartitionID, ShardID: pdr.ShardID.Key()}
			b.ShardState.States[key].nodeIDs = nil
			b.ShardState.States[key].trustBase = nil
			require.Equal(t, b, blocks[0])
		}
	})

	t.Run("history management", func(t *testing.T) {
		// when block is committed (new root block) older blocks are deleted
		db, err := NewBoltStorage(filepath.Join(tempDir, "history.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		// initially db is empty
		blocks, err := db.LoadBlocks()
		require.NoError(t, err)
		require.Empty(t, blocks)

		b := blockForRound(4)
		require.NoError(t, db.WriteBlock(b, false))
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		b = blockForRound(5)
		require.NoError(t, db.WriteBlock(b, false))
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		require.Len(t, blocks, 2)

		// write new root block, older blocks will be deleted
		b = blockForRound(6)
		b.CommitQc = &rctypes.QuorumCert{}
		require.NoError(t, db.WriteBlock(b, true))
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 1) {
			require.Equal(t, b.GetRound(), blocks[0].GetRound())
		}

		// add two non-root blocks
		b7 := blockForRound(7)
		require.NoError(t, db.WriteBlock(b7, false))
		b = blockForRound(8)
		require.NoError(t, db.WriteBlock(b, false))
		// check we do have expected blocks in DB
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 3) {
			// "contract" is that blocks are loaded in the descending order of round number
			require.EqualValues(t, 8, blocks[0].GetRound())
			require.EqualValues(t, 7, blocks[1].GetRound())
			require.EqualValues(t, 6, blocks[2].GetRound())
		}

		// commit round 7, the round 6 must go, round 8 must stay
		b7.CommitQc = &rctypes.QuorumCert{}
		require.NoError(t, db.WriteBlock(b7, true))
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 2) {
			require.EqualValues(t, 8, blocks[0].GetRound())
			require.EqualValues(t, 7, blocks[1].GetRound())
		}
		// and committing the same round again mustn't change the state in DB
		require.NoError(t, db.WriteBlock(b7, true))
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 2) {
			require.EqualValues(t, 8, blocks[0].GetRound())
			require.EqualValues(t, 7, blocks[1].GetRound())
		}
	})
}

func Test_BoltDB_VoteMsg(t *testing.T) {
	dir := t.TempDir()

	t.Run("invalid database", func(t *testing.T) {
		db, err := NewBoltStorage(filepath.Join(dir, "invalid_db.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		err = db.db.Update(func(tx *bbolt.Tx) error { return tx.DeleteBucket(bucketVotes) })
		require.NoError(t, err)

		vote := &abdrc.VoteMsg{}
		require.ErrorIs(t, db.WriteVote(vote), errNoVoteBucket)

		msg, err := db.ReadLastVote()
		require.ErrorIs(t, err, errNoVoteBucket)
		require.Nil(t, msg)
	})

	t.Run("invalid data", func(t *testing.T) {
		db, err := NewBoltStorage(filepath.Join(dir, "invalid_data.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		// invalid input data
		require.EqualError(t, db.WriteVote(42), `unknown vote type int`)
		require.EqualError(t, db.WriteVote(abdrc.ProposalMsg{}), `unknown vote type abdrc.ProposalMsg`)

		// set the vote in DB to invalid data
		// not correct data structure
		err = db.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketVotes)
			return b.Put(keyVote, []byte("foobar"))
		})
		require.NoError(t, err)
		msg, err := db.ReadLastVote()
		require.EqualError(t, err, `deserializing vote info: unexpected EOF`)
		require.Empty(t, msg)

		// invalid type enum value
		encoded, err := types.Cbor.Marshal(VoteStore{VoteType: 10})
		require.NoError(t, err)
		err = db.db.Update(func(tx *bbolt.Tx) error {
			b := tx.Bucket(bucketVotes)
			return b.Put(keyVote, encoded)
		})
		require.NoError(t, err)
		msg, err = db.ReadLastVote()
		require.EqualError(t, err, `unsupported vote kind: 10`)
		require.Empty(t, msg)
	})

	t.Run("success", func(t *testing.T) {
		db, err := NewBoltStorage(filepath.Join(dir, "votes.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		// empty db
		msg, err := db.ReadLastVote()
		require.NoError(t, err)
		require.Nil(t, msg)

		// vote
		vote := abdrc.VoteMsg{
			VoteInfo: &rctypes.RoundInfo{
				Version:     1,
				RoundNumber: 9872,
				Epoch:       1,
				Timestamp:   types.NewTimestamp(),
			},
			Author:    "node id",
			Signature: test.RandomBytes(32),
		}
		require.NoError(t, db.WriteVote(vote))
		msg, err = db.ReadLastVote()
		require.NoError(t, err)
		require.Equal(t, &vote, msg)

		// timeout vote
		voteTO := &abdrc.TimeoutMsg{
			Timeout: &rctypes.Timeout{
				Epoch: 1,
				Round: 10,
			},
			LastTC: &rctypes.TimeoutCert{
				Timeout:    &rctypes.Timeout{},
				Signatures: map[string]*rctypes.TimeoutVote{"foobar": {HqcRound: 8, Signature: []byte{5, 1, 0}}},
			},
			Author:    "test",
			Signature: test.RandomBytes(32),
		}
		require.NoError(t, db.WriteVote(voteTO))
		msg, err = db.ReadLastVote()
		require.NoError(t, err)
		require.Equal(t, voteTO, msg)
	})
}

func Test_BoltDB_TimeoutCert(t *testing.T) {
	dir := t.TempDir()

	t.Run("invalid database", func(t *testing.T) {
		db, err := NewBoltStorage(filepath.Join(dir, "invalid.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })
		err = db.db.Update(func(tx *bbolt.Tx) error { return tx.DeleteBucket(bucketCertificates) })
		require.NoError(t, err)

		tc := &rctypes.TimeoutCert{Timeout: rctypes.NewTimeout(7, 1, &rctypes.QuorumCert{})}
		require.ErrorIs(t, db.WriteTC(tc), errNoCertificatesBucket)

		tc, err = db.ReadLastTC()
		require.ErrorIs(t, err, errNoCertificatesBucket)
		require.Nil(t, tc)
	})

	t.Run("success", func(t *testing.T) {
		db, err := NewBoltStorage(filepath.Join(dir, "timeout_cert.db"))
		require.NoError(t, err)
		t.Cleanup(func() { db.Close() })

		tc, err := db.ReadLastTC()
		require.NoError(t, err)
		require.Nil(t, tc)

		tc = &rctypes.TimeoutCert{Timeout: rctypes.NewTimeout(7, 1, &rctypes.QuorumCert{})}
		require.NoError(t, db.WriteTC(tc))
		tc2, err := db.ReadLastTC()
		require.NoError(t, err)
		require.Equal(t, tc, tc2)

		// when there is a block for the TO round it should be deleted when storing TC
		b := ExecutedBlock{BlockData: &rctypes.BlockData{Round: 17}}
		require.NoError(t, db.WriteBlock(&b, false))
		b = ExecutedBlock{BlockData: &rctypes.BlockData{Round: 18}}
		require.NoError(t, db.WriteBlock(&b, false))
		blocks, err := db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 2) {
			require.EqualValues(t, 18, blocks[0].GetRound())
			require.EqualValues(t, 17, blocks[1].GetRound())
		}
		tc.Timeout.Round = b.GetRound()
		require.NoError(t, db.WriteTC(tc))
		tc2, err = db.ReadLastTC()
		require.NoError(t, err)
		require.Equal(t, tc, tc2)
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		if assert.Len(t, blocks, 1) {
			require.EqualValues(t, 17, blocks[0].GetRound())
		}
	})
}

func Test_BoltDB_SafetyModule_API(t *testing.T) {
	dbName := filepath.Join(t.TempDir(), "safety.db")
	db, err := NewBoltStorage(dbName)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// empty DB must have been initialized to genesis round
	require.Equal(t, rctypes.GenesisRootRound, db.GetHighestQcRound())
	require.Equal(t, rctypes.GenesisRootRound, db.GetHighestVotedRound())

	// set new values, voted round
	require.NoError(t, db.SetHighestVotedRound(10))
	require.EqualValues(t, 10, db.GetHighestVotedRound())
	// shouldn't be able to reset into the past
	require.NoError(t, db.SetHighestVotedRound(9))
	require.EqualValues(t, 10, db.GetHighestVotedRound())

	// set new values, high QC round (and voted round)
	require.NoError(t, db.SetHighestQcRound(20, 21))
	require.EqualValues(t, 20, db.GetHighestQcRound())
	require.EqualValues(t, 21, db.GetHighestVotedRound())
	// shouldn't be able to reset into the past
	require.NoError(t, db.SetHighestQcRound(10, 10))
	require.EqualValues(t, 20, db.GetHighestQcRound())
	require.EqualValues(t, 21, db.GetHighestVotedRound())
	// moving one forward mustn't allow to set other into past
	require.NoError(t, db.SetHighestQcRound(10, 22))
	require.EqualValues(t, 20, db.GetHighestQcRound())
	require.EqualValues(t, 22, db.GetHighestVotedRound())
	require.NoError(t, db.SetHighestQcRound(21, 10))
	require.EqualValues(t, 21, db.GetHighestQcRound())
	require.EqualValues(t, 22, db.GetHighestVotedRound())

	// close and reopen, should get back to the last state saved
	require.NoError(t, db.Close())
	db, err = NewBoltStorage(dbName)
	require.NoError(t, err)
	require.EqualValues(t, 21, db.GetHighestQcRound())
	require.EqualValues(t, 22, db.GetHighestVotedRound())

	// invalid db - missing bucket
	err = db.db.Update(func(tx *bbolt.Tx) error { return tx.DeleteBucket(bucketSafety) })
	require.NoError(t, err)
	require.Equal(t, rctypes.GenesisRootRound, db.GetHighestQcRound())
	require.Equal(t, rctypes.GenesisRootRound, db.GetHighestVotedRound())
	require.ErrorIs(t, db.SetHighestQcRound(20, 21), errNoSafetyBucket)
}
