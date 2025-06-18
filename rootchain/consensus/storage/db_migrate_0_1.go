package storage

import (
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	rctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

/*
init empty DB to version 1 structure (create buckets and default values)
*/
func initVersion1Buckets(tx *bbolt.Tx) error {
	if _, err := tx.CreateBucket(bucketBlocks); err != nil {
		return fmt.Errorf("creating bucket for blocks: %w", err)
	}
	if _, err := tx.CreateBucket(bucketCertificates); err != nil {
		return fmt.Errorf("creating bucket for certificates: %w", err)
	}
	if _, err := tx.CreateBucket(bucketVotes); err != nil {
		return fmt.Errorf("creating bucket for votes: %w", err)
	}

	b, err := tx.CreateBucket(bucketSafety)
	if err != nil {
		return fmt.Errorf("creating bucket for votes: %w", err)
	}
	if err = writeUint64(b, keyHighestQc, rctypes.GenesisRootRound); err != nil {
		return fmt.Errorf("storing highest QC round: %w", err)
	}
	if err = writeUint64(b, keyHighestVoted, rctypes.GenesisRootRound); err != nil {
		return fmt.Errorf("storing highest voted round: %w", err)
	}

	if _, err := tx.CreateBucket(bucketMetadata); err != nil {
		return fmt.Errorf("creating bucket for metadata: %w", err)
	}
	return setVersion(tx, 1)
}

/*
migrate_0_to_1 migrate "everything in 'default' bucket" to type safe storage implementation
*/
func (db BoltDB) migrate_0_to_1() error {
	return db.db.Update(func(tx *bbolt.Tx) error {
		// read current data
		b := tx.Bucket([]byte("default"))
		if b == nil {
			return errors.New(`the "default" bucket not found`)
		}
		var vote, tc, highQCR, highVR []byte
		var blocks []*ExecutedBlock
		err := b.ForEach(func(k, v []byte) error {
			switch keyStr := string(k); {
			case strings.HasPrefix(keyStr, "block_"):
				var b ExecutedBlock
				if err := cbor.Unmarshal(v, &b); err != nil {
					return fmt.Errorf("loading block %x: %w", k, err)
				}
				blocks = append(blocks, &b)
			case keyStr == string("vote"):
				vote = slices.Clone(v)
			case keyStr == string("tc"):
				tc = slices.Clone(v)
			case keyStr == string("votedRound"):
				highVR = slices.Clone(v)
			case keyStr == string("qcRound"):
				highQCR = slices.Clone(v)
			default:
				return fmt.Errorf("unknown key %x", k)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("loading current data: %w", err)
		}

		// create new DB bucket structure
		if err := initVersion1Buckets(tx); err != nil {
			return err
		}

		// move blocks into blocks bucket
		if b = tx.Bucket(bucketBlocks); b == nil {
			return fmt.Errorf("bucket %s doesn't exist", bucketBlocks)
		}
		for _, v := range blocks {
			data, err := cbor.Marshal(v)
			if err != nil {
				return fmt.Errorf("serializing block: %w", err)
			}
			if err := b.Put(binary.BigEndian.AppendUint64(make([]byte, 0, 8), v.GetRound()), data); err != nil {
				return fmt.Errorf("saving block %d: %w", v.GetRound(), err)
			}
		}

		// move votes
		if vote != nil {
			if b = tx.Bucket(bucketVotes); b == nil {
				return fmt.Errorf("bucket %s doesn't exist", bucketVotes)
			}
			if err := b.Put(keyVote, vote); err != nil {
				return fmt.Errorf("moving vote: %w", err)
			}
		}

		// move TC
		if tc != nil {
			if b = tx.Bucket(bucketCertificates); b == nil {
				return fmt.Errorf("bucket %s doesn't exist", bucketCertificates)
			}
			if err := b.Put(keyTimeoutCert, tc); err != nil {
				return fmt.Errorf("moving vote: %w", err)
			}
		}

		// move safety module state
		if highQCR != nil {
			if b = tx.Bucket(bucketSafety); b == nil {
				return fmt.Errorf("bucket %s doesn't exist", bucketSafety)
			}
			var round uint64
			if err := cbor.Unmarshal(highQCR, &round); err != nil {
				return fmt.Errorf("loading high QC round: %w", err)
			}
			if err = writeUint64(b, keyHighestQc, round); err != nil {
				return err
			}
		}
		if highVR != nil {
			if b = tx.Bucket(bucketSafety); b == nil {
				return fmt.Errorf("bucket %s doesn't exist", bucketSafety)
			}
			var round uint64
			if err := cbor.Unmarshal(highVR, &round); err != nil {
				return fmt.Errorf("loading high voted round: %w", err)
			}
			if err = writeUint64(b, keyHighestVoted, round); err != nil {
				return err
			}
		}

		// delete version 0 data
		return tx.DeleteBucket([]byte("default"))
	})
}
