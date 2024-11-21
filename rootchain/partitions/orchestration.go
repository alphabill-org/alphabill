package partitions

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	bolt "go.etcd.io/bbolt"
)

var (
	partitionsBucketName   = []byte("partition-bucket")
	roundToEpochBucketName = []byte("round-to-epoch-bucket")
	epochToVarBucketName   = []byte("epoch-to-var-bucket")
)

type (
	Orchestration struct {
		db   *bolt.DB
		seed *genesis.RootGenesis
	}
)

/*
NewOrchestration creates new boltDB implementation of shard validator orchestration.
  - seed is the root genesis file used to generate db schema and the first VARs.
  - dbFile is filename (full path) to the Bolt DB file to use for storage,
    if the file does not exist it will be created;
*/
func NewOrchestration(seed *genesis.RootGenesis, dbFile string) (*Orchestration, error) {
	if seed == nil {
		return nil, errors.New("root genesis file is required")
	}
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt DB: %w", err)
	}
	if err = createSchemaAndFirstVAR(db, seed); err != nil {
		return nil, fmt.Errorf("initializing the DB: %w", err)
	}
	return &Orchestration{db: db, seed: seed}, nil
}

// ShardEpoch returns the active epoch number for the given round in a shard.
func (o *Orchestration) ShardEpoch(partition types.PartitionID, shard types.ShardID, shardRound uint64) (uint64, error) {
	var epoch uint64
	err := o.db.View(func(tx *bolt.Tx) error {
		roundToEpochBucket, _, err := shardBuckets(tx, partition, shard)
		if err != nil {
			return err
		}

		c := roundToEpochBucket.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			epoch = keyToUint64(v)
			epochStartRound := keyToUint64(k)
			if epochStartRound <= shardRound {
				return nil
			}
		}
		return errors.New("epoch not found (db is empty?)")
	})
	if err != nil {
		return 0, fmt.Errorf("db tx failed: %w", err)
	}
	return epoch, nil
}

// ShardConfig returns VAR for the given shard epoch.
func (o *Orchestration) ShardConfig(partition types.PartitionID, shard types.ShardID, epoch uint64) (*ValidatorAssignmentRecord, error) {
	var rec *ValidatorAssignmentRecord
	err := o.db.View(func(tx *bolt.Tx) error {
		var err error
		rec, err = getVAR(tx, partition, shard, epoch)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load VAR for partition %q shard %q and epoch %q: %w", partition, shard, epoch, err)
	}
	return rec, nil
}

// AddShardConfig verifies and stores the given VAR.
//
// Validation rules:
//   - The network ID must match
//   - The partition ID must match one of the existing partitions
//   - The shard ID must be 0x80 (CBOR encoding of the empty bitstring)
//   - The new epoch number must be one greater than the current epoch of the only shard in the specified partition
//   - The activation round number must be strictly greater than the current round of the only shard in the specified partition
//   - The node identifiers must match their authentication keys
func (o *Orchestration) AddShardConfig(rec *ValidatorAssignmentRecord) error {
	err := o.db.Update(func(tx *bolt.Tx) error {
		if err := verifyVAR(tx, rec); err != nil {
			return fmt.Errorf("verify var: %w", err)
		}
		if err := setVAR(tx, rec); err != nil {
			return fmt.Errorf("set var: %w", err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("tx failed: %w", err)
	}
	return nil
}

func (o *Orchestration) RoundPartitions(rootRound uint64) ([]*genesis.GenesisPartitionRecord, error) {
	return o.seed.Partitions, nil
}

func (o *Orchestration) PartitionGenesis(partitionID types.PartitionID) (*genesis.GenesisPartitionRecord, error) {
	for _, pg := range o.seed.Partitions {
		if pg.PartitionDescription.PartitionIdentifier == partitionID {
			return pg, nil
		}
	}
	return nil, fmt.Errorf("partition genesis not found for partition %d", partitionID)
}

func (o *Orchestration) Close() error {
	return o.db.Close()
}

func getVAR(tx *bolt.Tx, partition types.PartitionID, shard types.ShardID, epoch uint64) (*ValidatorAssignmentRecord, error) {
	_, epochToVarBucket, err := shardBuckets(tx, partition, shard)
	if err != nil {
		return nil, err
	}
	varBytes := epochToVarBucket.Get(uint64ToKey(epoch))
	if varBytes == nil {
		return nil, fmt.Errorf("the epoch %d does not exist", epoch)
	}
	var rec *ValidatorAssignmentRecord
	if err := json.Unmarshal(varBytes, &rec); err != nil {
		return nil, fmt.Errorf("failed to parse VAR json: %w", err)
	}
	return rec, nil
}

func setVAR(tx *bolt.Tx, rec *ValidatorAssignmentRecord) error {
	varBytes, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("failed to parse VAR json: %w", err)
	}
	roundToEpochBucket, epochToVarBucket, err := shardBuckets(tx, rec.PartitionID, rec.ShardID)
	if err != nil {
		return err
	}
	if err = roundToEpochBucket.Put(uint64ToKey(rec.RoundNumber), uint64ToKey(rec.EpochNumber)); err != nil {
		return fmt.Errorf("storing round to epoch index: %w", err)
	}
	if err = epochToVarBucket.Put(uint64ToKey(rec.EpochNumber), varBytes); err != nil {
		return fmt.Errorf("storing var: %w", err)
	}
	return nil
}

func verifyVAR(tx *bolt.Tx, rec *ValidatorAssignmentRecord) error {
	// currently every VAR must extend previous VAR (no adding of new partitions/shards)
	if rec.EpochNumber == 0 {
		return errors.New("invalid epoch number, must not be zero")
	}
	previousVAR, err := getVAR(tx, rec.PartitionID, rec.ShardID, rec.EpochNumber-1)
	if err != nil {
		return fmt.Errorf("previous var not found: %w", err)
	}
	if err = rec.Verify(previousVAR); err != nil {
		return fmt.Errorf("var does not extend previous var: %w", err)
	}
	return err
}

func shardBuckets(tx *bolt.Tx, partition types.PartitionID, shard types.ShardID) (*bolt.Bucket, *bolt.Bucket, error) {
	partitionsBucket := tx.Bucket(partitionsBucketName)
	if partitionsBucket == nil {
		return nil, nil, fmt.Errorf("the partitions root bucket %q does not exist", partitionsBucketName)
	}
	partitionBucket := partitionsBucket.Bucket(partition.Bytes())
	if partitionBucket == nil {
		return nil, nil, fmt.Errorf("the partition 0x%x does not exist", partition.Bytes())
	}
	shardBucket := partitionBucket.Bucket(shard.Bytes())
	if shardBucket == nil {
		return nil, nil, fmt.Errorf("the partition shard 0x%x does not exist", shard.Bytes())
	}
	epochToVarBucket := shardBucket.Bucket(epochToVarBucketName)
	if epochToVarBucket == nil {
		return nil, nil, fmt.Errorf("the epoch to var bucket %q does not exist", epochToVarBucketName)
	}
	roundToEpochBucket := shardBucket.Bucket(roundToEpochBucketName)
	if roundToEpochBucket == nil {
		return nil, nil, fmt.Errorf("the round to epoch bucket %q does not exist", roundToEpochBucketName)
	}
	return roundToEpochBucket, epochToVarBucket, nil
}

func createSchemaAndFirstVAR(db *bolt.DB, seed *genesis.RootGenesis) error {
	// schema:
	// partitions bucket (root bucket)
	//   multiple partition buckets (partition id to shard bucket)
	//     multiple shard buckets (shard id to shard bucket)
	//       shard round to epoch bucket
	//       epoch to var bucket
	return db.Update(func(tx *bolt.Tx) error {
		// skip schema creation if it already exists
		partitionsBucket := tx.Bucket(partitionsBucketName)
		if partitionsBucket != nil {
			return nil
		}

		partitionsBucket, err := tx.CreateBucketIfNotExists(partitionsBucketName)
		if err != nil {
			return fmt.Errorf("creating the root %q bucket: %w", partitionsBucketName, err)
		}
		for _, partition := range seed.Partitions {
			rec := NewVARFromGenesis(partition)
			partitionBucket, err := partitionsBucket.CreateBucketIfNotExists(rec.PartitionID.Bytes())
			if err != nil {
				return fmt.Errorf("creating the partition %q bucket: %w", rec.PartitionID.Bytes(), err)
			}
			shardBucket, err := partitionBucket.CreateBucketIfNotExists(rec.ShardID.Bytes())
			if err != nil {
				return fmt.Errorf("creating the shard %q bucket: %w", rec.ShardID.Bytes(), err)
			}
			roundToEpochBucket, err := shardBucket.CreateBucketIfNotExists(roundToEpochBucketName)
			if err != nil {
				return fmt.Errorf("creating the round to epoch %q bucket: %w", roundToEpochBucketName, err)
			}
			epochToVarBucket, err := shardBucket.CreateBucketIfNotExists(epochToVarBucketName)
			if err != nil {
				return fmt.Errorf("creating the epoch to var %q bucket: %w", epochToVarBucketName, err)
			}
			if err = roundToEpochBucket.Put(uint64ToKey(rec.RoundNumber), uint64ToKey(rec.EpochNumber)); err != nil {
				return fmt.Errorf("storing round to epoch index: %w", err)
			}
			varBytes, err := json.Marshal(rec)
			if err != nil {
				return fmt.Errorf("marshalling var to json: %w", err)
			}
			if err = epochToVarBucket.Put(uint64ToKey(rec.EpochNumber), varBytes); err != nil {
				return fmt.Errorf("storing var: %w", err)
			}
		}
		return nil
	})
}

func uint64ToKey(n uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, n)
	return key
}

func keyToUint64(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
}
