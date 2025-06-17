package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

var (
	bucketBlocks       = []byte("blocks")
	bucketCertificates = []byte("certificates")
	bucketVotes        = []byte("votes")
	bucketSafety       = []byte("safety") // safety module state
	bucketMetadata     = []byte("metadata")

	keyDbVersion    = []byte("version")
	keyTimeoutCert  = []byte("tc")
	keyVote         = []byte("vote")
	keyHighestVoted = []byte("votedRound")
	keyHighestQc    = []byte("qcRound")
)

/*
Implementation of persistent storage for the BlockTree and SafetyModule using bbolt database.
*/
type BoltDB struct {
	db *bbolt.DB
}

func NewBoltStorage(file string) (db BoltDB, err error) {
	_, err = os.Stat(file)
	newDB := err != nil && errors.Is(err, fs.ErrNotExist)

	db.db, err = bbolt.Open(file, 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return db, fmt.Errorf("open database: %w", err)
	}
	defer func() {
		if err != nil {
			_ = db.db.Close()
			db.db = nil
		}
	}()

	if newDB {
		if err := db.db.Update(initVersion1Buckets); err != nil {
			return db, fmt.Errorf("initializing new database: %w", err)
		}
	} else {
		if err := db.migrateTo(1); err != nil {
			return db, fmt.Errorf("upgrading DB version: %w", err)
		}
	}

	return db, nil
}

func (db BoltDB) Close() error { return db.db.Close() }

var errNoBlocksBucket = errors.New("blocks bucket not found")

/*
LoadBlocks returns all the blocks in the database.
The list is sorted in descending order of round number.
*/
func (db BoltDB) LoadBlocks() (blocks []*ExecutedBlock, err error) {
	return blocks, db.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		if b == nil {
			return errNoBlocksBucket
		}

		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var b ExecutedBlock
			if err := cbor.Unmarshal(v, &b); err != nil {
				return fmt.Errorf("loading block %x: %w", k, err)
			}
			blocks = append(blocks, &b)
		}
		return nil
	})
}

/*
WriteBlock stores "block" into database. If "root" is "true" older
blocks (based on round number) will be deleted.
*/
func (db BoltDB) WriteBlock(block *ExecutedBlock, root bool) error {
	data, err := cbor.Marshal(block)
	if err != nil {
		return fmt.Errorf("serializing block: %w", err)
	}
	key := binary.BigEndian.AppendUint64(make([]byte, 0, 8), block.GetRound())

	return db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketBlocks)
		if b == nil {
			return errNoBlocksBucket
		}
		if err := b.Put(key, data); err != nil {
			return fmt.Errorf("storing block: %w", err)
		}

		if !root {
			return nil
		}
		if block.CommitQc == nil {
			return errors.New("root block must have commit QC")
		}

		// we do not keep history so anything older than the root can be deleted
		c := b.Cursor()
		if k, _ := c.Seek(key); !bytes.Equal(k, key) {
			return fmt.Errorf("seeking %x but landed on %x", key, k)
		}
		for k, _ := c.Prev(); k != nil; k, _ = c.Prev() {
			if err := c.Delete(); err != nil {
				return fmt.Errorf("delete key %x: %w", k, err)
			}
		}
		return nil
	})
}

var errNoCertificatesBucket = errors.New("certificates bucket not found")

func (db BoltDB) WriteTC(tc *rctypes.TimeoutCert) error {
	data, err := cbor.Marshal(tc)
	if err != nil {
		return fmt.Errorf("serializing TimeoutCert: %w", err)
	}
	blockKey := binary.BigEndian.AppendUint64(make([]byte, 0, 8), tc.GetRound())

	return db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketCertificates)
		if b == nil {
			return errNoCertificatesBucket
		}
		if err := b.Put(keyTimeoutCert, data); err != nil {
			return err
		}
		// if there is block for the round deleted it
		if b = tx.Bucket(bucketBlocks); b == nil {
			return errNoBlocksBucket
		}
		return b.Delete(blockKey)
	})
}

func (db BoltDB) ReadLastTC() (tc *rctypes.TimeoutCert, _ error) {
	// we currently only keep the latest TC, stored to [bucketCertificates -> keyTimeoutCert]
	return tc, db.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketCertificates)
		if b == nil {
			return errNoCertificatesBucket
		}
		if k, v := b.Cursor().Seek(keyTimeoutCert); bytes.Equal(k, keyTimeoutCert) {
			return cbor.Unmarshal(v, &tc)
		}
		return nil
	})
}

var errNoVoteBucket = errors.New("vote bucket not found")

const (
	Unknown VoteType = iota
	VoteMsg
	TimeoutVoteMsg
)

type VoteType uint8

type VoteStore struct {
	VoteType VoteType
	VoteMsg  cbor.RawCBOR
}

func (db BoltDB) WriteVote(vote any) (err error) {
	voteInfo := VoteStore{}
	switch vote.(type) {
	case *abdrc.VoteMsg, abdrc.VoteMsg:
		voteInfo.VoteType = VoteMsg
	case *abdrc.TimeoutMsg, abdrc.TimeoutMsg:
		voteInfo.VoteType = TimeoutVoteMsg
	default:
		return fmt.Errorf("unknown vote type %T", vote)
	}

	if voteInfo.VoteMsg, err = cbor.Marshal(vote); err != nil {
		return fmt.Errorf("vote message serialization failed: %w", err)
	}

	encoded, err := cbor.Marshal(voteInfo)
	if err != nil {
		return fmt.Errorf("vote info serialization failed: %w", err)
	}

	return db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketVotes)
		if b == nil {
			return errNoVoteBucket
		}
		return b.Put(keyVote, encoded)
	})
}

func (db BoltDB) ReadLastVote() (msg any, err error) {
	return msg, db.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketVotes)
		if b == nil {
			return errNoVoteBucket
		}

		voteInfo := VoteStore{}
		if k, v := b.Cursor().Seek(keyVote); bytes.Equal(k, keyVote) {
			if err := cbor.Unmarshal(v, &voteInfo); err != nil {
				return fmt.Errorf("deserializing vote info: %w", err)
			}
		} else {
			return nil
		}

		switch voteInfo.VoteType {
		case VoteMsg:
			msg = &abdrc.VoteMsg{}
		case TimeoutVoteMsg:
			msg = &abdrc.TimeoutMsg{}
		default:
			return fmt.Errorf("unsupported vote kind: %d", voteInfo.VoteType)
		}
		if err := cbor.Unmarshal(voteInfo.VoteMsg, msg); err != nil {
			return fmt.Errorf("deserializing vote message (%T): %w", msg, err)
		}
		return nil
	})
}

var errNoSafetyBucket = errors.New("safety module bucket not found")

func (db BoltDB) GetHighestVotedRound() (round uint64) {
	err := db.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSafety)
		if b == nil {
			return errNoSafetyBucket
		}
		var err error
		round, err = readUint64(b, keyHighestVoted)
		return err
	})
	if err != nil {
		return rctypes.GenesisRootRound
	}
	return round
}

func (db BoltDB) SetHighestVotedRound(round uint64) error {
	return db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSafety)
		if b == nil {
			return errNoSafetyBucket
		}
		hVR, err := readUint64(b, keyHighestVoted)
		if err != nil {
			return err
		}
		return writeUint64(b, keyHighestVoted, max(round, hVR))
	})
}

func (db BoltDB) GetHighestQcRound() (round uint64) {
	err := db.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSafety)
		if b == nil {
			return errNoSafetyBucket
		}
		var err error
		round, err = readUint64(b, keyHighestQc)
		return err
	})
	if err != nil {
		return rctypes.GenesisRootRound
	}
	return round
}

func (db BoltDB) SetHighestQcRound(qcRound, votedRound uint64) error {
	return db.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket(bucketSafety)
		if b == nil {
			return errNoSafetyBucket
		}
		hQC, err := readUint64(b, keyHighestQc)
		if err != nil {
			return err
		}
		hVR, err := readUint64(b, keyHighestVoted)
		if err != nil {
			return err
		}
		/* should we return error when attempting to set earlier round? ie
		if hQC < qcRound {
			return fmt.Errorf("attempt to reset QC round back - stored %d, proposed new %d", hQC, qcRound)
		}*/

		if err = writeUint64(b, keyHighestQc, max(qcRound, hQC)); err != nil {
			return err
		}
		return writeUint64(b, keyHighestVoted, max(votedRound, hVR))
	})
}

/*
migrateTo upgrades database to version "ver" if the current version is older.
*/
func (db BoltDB) migrateTo(ver uint64) error {
	curVer, err := db.getVersion()
	if err != nil {
		return fmt.Errorf("determining current version of the database: %w", err)
	}
	if curVer > ver {
		return fmt.Errorf("downgrading database version not supported, current is %d, asking for %d", curVer, ver)
	}

	for ; curVer < ver; curVer++ {
		switch curVer {
		case 0:
			err = db.migrate_0_to_1()
		default:
			return fmt.Errorf("migration from version %d to the next version not implemented", curVer)
		}

		if err != nil {
			return fmt.Errorf("migrating from version %d: %w", curVer, err)
		}
	}
	return nil
}

func (db BoltDB) getVersion() (ver uint64, _ error) {
	return ver, db.db.View(func(tx *bbolt.Tx) (err error) {
		b := tx.Bucket(bucketMetadata)
		if b == nil {
			// no bucket, must be version 0 database
			return nil
		}
		ver, err = readUint64(b, keyDbVersion)
		return err
	})
}

func setVersion(tx *bbolt.Tx, version uint64) error {
	b := tx.Bucket(bucketMetadata)
	if b == nil {
		return errors.New("metadata bucket not found")
	}
	return writeUint64(b, keyDbVersion, version)
}

func readUint64(b *bbolt.Bucket, key []byte) (uint64, error) {
	v := b.Get(key)
	if v == nil {
		return 0, fmt.Errorf("key %x not found", key)
	}
	if len(v) != 8 {
		return 0, fmt.Errorf("expected value of the %x to be 8 bytes, got %d", key, len(v))
	}
	return binary.BigEndian.Uint64(v), nil
}

func writeUint64(b *bbolt.Bucket, key []byte, value uint64) error {
	return b.Put(key, binary.BigEndian.AppendUint64(nil, value))
}
