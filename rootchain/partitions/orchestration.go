package partitions

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	bolt "go.etcd.io/bbolt"
)

var rootBucketName = []byte("root")

type (
	Orchestration struct {
		networkID types.NetworkID
		db        *bolt.DB
		log       *slog.Logger
	}
)

/*
NewOrchestration creates new boltDB implementation of shard validator orchestration.
  - dbFile is filename (full path) to the Bolt DB file to use for storage,
    if the file does not exist it will be created;
*/
func NewOrchestration(networkID types.NetworkID, dbFile string, log *slog.Logger) (*Orchestration, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt DB: %w", err)
	}

	// ensure root bucket exists
	err = db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(rootBucketName)
		if err != nil {
			return fmt.Errorf("creating %q bucket: %w", rootBucketName, err)
		}
		return nil
	})

	return &Orchestration{
		networkID: networkID,
		db:        db,
		log:       log,
	}, nil
}

func (o *Orchestration) NetworkID() types.NetworkID {
	return o.networkID
}

// ShardConfig returns ShardConf for the given root round.
func (o *Orchestration) ShardConfig(partitionID types.PartitionID, shardID types.ShardID, rootRound uint64) (*types.PartitionDescriptionRecord, error) {
	var shardConf *types.PartitionDescriptionRecord
	err := o.db.View(func(tx *bolt.Tx) error {
		var err error
		shardConf, err = getShardConf(tx, partitionID, shardID, rootRound)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load shard conf for shard %s_%s: %w", partitionID, shardID.String(), err)
	}
	if shardConf == nil {
		return nil, fmt.Errorf("shard conf missing for shard %s_%s", partitionID, shardID.String())
	}
	if shardConf.NetworkID != o.networkID {
		return nil, fmt.Errorf("shard conf loaded from database has wrong netorkID %d, expected %d", shardConf.NetworkID, o.networkID)
	}
	return shardConf, nil
}

/*
ShardConfigs returns shard confs active in the given root round.
*/
func (o *Orchestration) ShardConfigs(rootRound uint64) (map[types.PartitionShardID]*types.PartitionDescriptionRecord, error) {
	shardConfs := make(map[types.PartitionShardID]*types.PartitionDescriptionRecord)

	err := o.db.View(func(tx *bolt.Tx) error {
		rootBucket := tx.Bucket(rootBucketName)
		if rootBucket == nil {
			return fmt.Errorf("bucket %q does not exist", rootBucketName)
		}

		// check all partitions
		return rootBucket.ForEachBucket(func(partitionID []byte) error {
			partitionBucket := rootBucket.Bucket(partitionID)

			// check all shards
			return partitionBucket.ForEachBucket(func(shardID []byte) error {
				shardBucket := partitionBucket.Bucket(shardID)

				// check if there is an active shard conf for the given root round
				c := shardBucket.Cursor()
				for k, v := c.Last(); k != nil; k, v = c.Prev() {
					epochStartRound := keyToUint64(k)
					if epochStartRound > rootRound {
						continue
					}

					var shardConf *types.PartitionDescriptionRecord
					if err := json.Unmarshal(v, &shardConf); err != nil {
						return fmt.Errorf("failed to unmarshal shard conf: %w", err)
					}
					if len(shardConf.Validators) == 0 {
						// empty validators list is signal that shard has been deleted,
						// do not return deleted shard config
						break
					}
					ps := types.PartitionShardID{
						PartitionID: shardConf.PartitionID,
						ShardID:     shardConf.ShardID.Key(),
					}
					shardConfs[ps] = shardConf

					// active shard conf found, look no further for this shard
					break
				}
				return nil
			})
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read shard confs: %w", err)
	}

	return shardConfs, nil
}

// AddShardConfig verifies and stores the given shard conf.
//
// Validation rules:
//   - The network ID must match
//   - The partition ID must match one of the existing partitions
//   - The shard ID must be 0x80 (CBOR encoding of the empty bitstring)
//   - The new epoch number must be one greater than the current epoch of the only shard in the specified partition
//   - The activation round number must be strictly greater than the current round of the only shard in the specified partition
//   - The node identifiers must match their authentication keys
func (o *Orchestration) AddShardConfig(shardConf *types.PartitionDescriptionRecord) error {
	if shardConf.NetworkID != o.networkID {
		return fmt.Errorf("invalid networkID %d, expected %d", shardConf.NetworkID, o.networkID)
	}
	err := o.db.Update(func(tx *bolt.Tx) error {
		if err := verifyShardConf(tx, shardConf); err != nil {
			return fmt.Errorf("verify shard conf: %w", err)
		}
		if err := storeShardConf(tx, shardConf); err != nil {
			return fmt.Errorf("store shard conf: %w", err)
		}
		return nil
	})
	if err != nil {
		o.log.Error(fmt.Sprintf("Failed to add shard config for partition %d, epoch %d",
			shardConf.PartitionID, shardConf.Epoch), logger.Error(err))
		return err
	}
	o.log.Info(fmt.Sprintf("Added shard config for partition %d, epoch %d, epoch start %d",
		shardConf.PartitionID, shardConf.Epoch, shardConf.EpochStart), logger.Error(err))
	return err
}

func (o *Orchestration) Close() error {
	return o.db.Close()
}

func getShardConf(tx *bolt.Tx, partitionID types.PartitionID, shardID types.ShardID, rootRound uint64) (*types.PartitionDescriptionRecord, error) {
	shardBucket := getShardBucket(tx, partitionID, shardID)
	if shardBucket == nil {
		return nil, nil
	}

	c := shardBucket.Cursor()
	for k, v := c.Last(); k != nil; k, v = c.Prev() {
		epochStartRound := keyToUint64(k)
		if epochStartRound > rootRound {
			continue
		}

		var shardConf *types.PartitionDescriptionRecord
		if err := json.Unmarshal(v, &shardConf); err != nil {
			return nil, fmt.Errorf("failed to unmarshal shard conf: %w", err)
		}
		return shardConf, nil
	}

	return nil, nil
}

func storeShardConf(tx *bolt.Tx, shardConf *types.PartitionDescriptionRecord) error {
	shardConfBytes, err := json.Marshal(shardConf)
	if err != nil {
		return fmt.Errorf("failed to marshal shard conf to json: %w", err)
	}

	roundToShardConfBucket, err := createShardBuckets(tx, shardConf.PartitionID, shardConf.ShardID)
	if err != nil {
		return err
	}

	if err = roundToShardConfBucket.Put(uint64ToKey(shardConf.EpochStart), shardConfBytes); err != nil {
		return fmt.Errorf("storing shard conf: %w", err)
	}
	return nil
}

func verifyShardConf(tx *bolt.Tx, shardConf *types.PartitionDescriptionRecord) error {
	if shardConf.Epoch == 0 {
		return shardConf.IsValid()
	}

	lastShardConf, err := getShardConf(tx, shardConf.PartitionID, shardConf.ShardID, math.MaxUint64)
	if err != nil {
		return fmt.Errorf("failed to get previous shard conf: %w", err)
	}
	if lastShardConf == nil {
		return fmt.Errorf("previous shard conf not found")
	}
	if err = shardConf.Verify(lastShardConf); err != nil {
		return fmt.Errorf("shard conf does not extend previous shard conf: %w", err)
	}
	return err
}

// schema:
// root bucket (root bucket)
//
//	multiple partition buckets (partition id to partition bucket)
//	  multiple shard buckets (shard id to shard bucket)
func createShardBuckets(tx *bolt.Tx, partitionID types.PartitionID, shardID types.ShardID) (*bolt.Bucket, error) {
	rootBucket := tx.Bucket(rootBucketName)
	if rootBucket == nil {
		return nil, fmt.Errorf("bucket %q does not exist", rootBucketName)
	}
	partitionBucket, err := rootBucket.CreateBucketIfNotExists(partitionID.Bytes())
	if err != nil {
		return nil, fmt.Errorf("creating partition 0x%x bucket: %w", partitionID.Bytes(), err)
	}
	shardBucket, err := partitionBucket.CreateBucketIfNotExists(shardID.Bytes())
	if err != nil {
		return nil, fmt.Errorf("creating shard 0x%x bucket: %w", shardID.Bytes(), err)
	}
	return shardBucket, nil
}

func getShardBucket(tx *bolt.Tx, partitionID types.PartitionID, shardID types.ShardID) *bolt.Bucket {
	rootBucket := tx.Bucket(rootBucketName)
	if rootBucket == nil {
		return nil
	}
	partitionBucket := rootBucket.Bucket(partitionID.Bytes())
	if partitionBucket == nil {
		return nil
	}
	return partitionBucket.Bucket(shardID.Bytes())
}

func uint64ToKey(n uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, n)
	return key
}

func keyToUint64(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
}
