package storage

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	"github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

//go:embed testdata/*.db
var testdata embed.FS

func Test_migrate_rootchain_0_to_1(t *testing.T) {
	dir := t.TempDir()

	t.Run("invalid source database", func(t *testing.T) {
		dbName := filepath.Join(dir, "no_bucket.db")
		// create empty bolt DB
		bDB, err := bbolt.Open(dbName, 0600, &bbolt.Options{Timeout: time.Second})
		require.NoError(t, err)
		require.NoError(t, bDB.Close())

		/*** no default bucket - file exist so we also expect it to contain data ***/
		db, err := NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: migrating from version 0: the "default" bucket not found`)
		require.Nil(t, db.db)

		/*** version 2 db ***/
		bDB, err = bbolt.Open(dbName, 0600, &bbolt.Options{Timeout: time.Second})
		require.NoError(t, err)
		err = bDB.Update(func(tx *bbolt.Tx) error {
			if _, err := tx.CreateBucket(bucketMetadata); err != nil {
				return err
			}
			return setVersion(tx, 2)
		})
		require.NoError(t, err)
		require.NoError(t, bDB.Close())

		db, err = NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: downgrading database version not supported, current is 2, asking for 1`)
		require.Nil(t, db.db)
	})

	t.Run("version 1 bucket already exists", func(t *testing.T) {
		dbName := filepath.Join(dir, "bucket_exists.db")
		// create new db with ver 1 structure
		db, err := NewBoltStorage(dbName)
		require.NoError(t, err)
		// to trigger upgrade on (next) open delete metadata bucket and add "default"
		err = db.db.Update(func(tx *bbolt.Tx) error {
			if _, err := tx.CreateBucket([]byte("default")); err != nil {
				return err
			}
			return tx.DeleteBucket(bucketMetadata)
		})
		require.NoError(t, err)
		require.NoError(t, db.Close())

		for _, bucketName := range [][]byte{bucketBlocks, bucketCertificates, bucketVotes, bucketSafety} {
			_, err := NewBoltStorage(dbName)
			require.ErrorIs(t, err, bbolt.ErrBucketExists)
			// now delete this bucket to trigger error on the next bucket which shouldn't exist
			bDB, err := bbolt.Open(dbName, 0600, &bbolt.Options{Timeout: time.Second})
			require.NoError(t, err)
			err = bDB.Update(func(tx *bbolt.Tx) error {
				return tx.DeleteBucket(bucketName)
			})
			require.NoError(t, err)
			require.NoError(t, bDB.Close())
		}
	})

	// make a copy of known good version 0 database and return it's name (path)
	ver0db := func(t *testing.T, fileName string) string {
		b, err := testdata.ReadFile("testdata/rootchain_v0.db")
		require.NoError(t, err)
		dbName := filepath.Join(dir, fileName)
		f, err := os.Create(dbName)
		require.NoError(t, err)
		_, err = f.Write(b)
		require.NoError(t, err)
		return dbName
	}

	t.Run("invalid data", func(t *testing.T) {
		// attempts to migrate version 0 DB which contains invalid data - we start
		// with known good ver 0 DB and make different keys invalid
		dbName := ver0db(t, "invData.db")

		makeInvalid := func(action func(b *bbolt.Bucket) error) {
			bDB, err := bbolt.Open(dbName, 0600, &bbolt.Options{Timeout: time.Second})
			require.NoError(t, err)
			require.NoError(t, bDB.Update(func(tx *bbolt.Tx) error {
				b := tx.Bucket([]byte("default"))
				return action(b)
			}))
			require.NoError(t, bDB.Close())
		}

		// ver 0 DB stores "votedRound" as CBOR encoded int
		makeInvalid(func(b *bbolt.Bucket) error {
			return b.Put([]byte("votedRound"), []byte("not CBOR int"))
		})
		_, err := NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: migrating from version 0: loading high voted round: unexpected EOF`)

		// ver 0 DB stores "qcRound" as CBOR encoded int
		makeInvalid(func(b *bbolt.Bucket) error {
			return b.Put([]byte("qcRound"), []byte{0x81, 0})
		})
		_, err = NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: migrating from version 0: loading high QC round: cbor: cannot unmarshal array into Go value of type uint64`)

		// unknown key
		makeInvalid(func(b *bbolt.Bucket) error {
			return b.Put([]byte("unknown key"), []byte{0, 0, 0, 0})
		})
		_, err = NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: migrating from version 0: loading current data: unknown key 756e6b6e6f776e206b6579`)

		// invalid block data in the ver 0 DB
		makeInvalid(func(b *bbolt.Bucket) error {
			k := append([]byte("block_"), 0, 0, 0, 0, 0, 0x35, 0x9d, 0xbf)
			return b.Put(k, []byte("not CBOR of a block"))
		})
		_, err = NewBoltStorage(dbName)
		require.EqualError(t, err, `upgrading DB version: migrating from version 0: loading current data: loading block 626c6f636b5f0000000000359dbf: cbor: 4 bytes of extraneous data starting at index 15`)
	})

	t.Run("success", func(t *testing.T) {
		// make a copy of known version 0 database
		dbName := ver0db(t, "success.db")
		// migrate it to ver 1
		db, err := NewBoltStorage(dbName)
		require.NoError(t, err)
		defer db.Close()
		// the DB version should be 1
		ver, err := db.getVersion()
		require.NoError(t, err)
		require.EqualValues(t, 1, ver, "DB version")
		// bucket "default" must be gone
		err = db.db.View(func(tx *bbolt.Tx) error {
			if tx.Bucket([]byte("default")) != nil {
				return errors.New("the 'default' bucket is still present")
			}
			return nil
		})
		require.NoError(t, err)

		/*** check do we have all the expected data (ie migration from 0 to 1 was a success) ***/

		require.EqualValues(t, 3513791, db.GetHighestQcRound())
		require.EqualValues(t, 3513792, db.GetHighestVotedRound())

		msg, err := db.ReadLastVote()
		require.NoError(t, err)
		vm, ok := msg.(*abdrc.VoteMsg)
		require.True(t, ok, "expected VoteMsg, got %T", msg)
		require.Equal(t, db.GetHighestVotedRound(), vm.VoteInfo.RoundNumber)

		msg, err = db.ReadLastTC()
		require.NoError(t, err)
		tc, ok := msg.(*types.TimeoutCert)
		require.True(t, ok, "expected TimeoutCert, got %T", msg)
		require.EqualValues(t, 3354117, tc.GetRound())

		blocks, err := db.LoadBlocks()
		require.NoError(t, err)
		require.Len(t, blocks, 67, "expected that there is 67 blocks in the DB")
		// find the root block
		idx := slices.IndexFunc(blocks, func(b *ExecutedBlock) bool { return b.CommitQc != nil })
		require.Equal(t, 2, idx)
		require.EqualValues(t, 3513790, blocks[idx].GetRound())

		// write committed block into DB again to trigger cleanup
		require.NoError(t, db.WriteBlock(blocks[idx], true))
		// now should have only three blocks left
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		require.Len(t, blocks, 3)
		require.EqualValues(t, 3513792, blocks[0].GetRound())
		require.EqualValues(t, 3513791, blocks[1].GetRound())
		require.EqualValues(t, 3513790, blocks[2].GetRound())

		/* reopen the DB, should be valid ver 1 DB containing 3 blocks */
		require.NoError(t, db.Close())
		db, err = NewBoltStorage(dbName)
		require.NoError(t, err)
		defer db.Close()
		blocks, err = db.LoadBlocks()
		require.NoError(t, err)
		require.Len(t, blocks, 3)
		require.EqualValues(t, 3513792, blocks[0].GetRound())
		require.EqualValues(t, 3513791, blocks[1].GetRound())
		require.EqualValues(t, 3513790, blocks[2].GetRound())
	})
}
