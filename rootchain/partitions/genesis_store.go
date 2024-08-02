package partitions

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

var rootGenesisBucket = []byte("root-genesis")

/*
genesisStore is persistent storage for root genesis files.

Bucket "root-genesis" maps round number (uint64) to root genesis file. The idea
is that starting with given round number the configuration described by the genesis
should be used.
*/
type genesisStore struct {
	db *bolt.DB
}

/*
  - dbFile is filename (full path) to the Bolt DB file to use for storage, if the
    file does not exist it will be created;
  - seed is stored into the DB only if the DB is empty, it should be the initial genesis
    of the rootchain (ie for round 1, the round number used is what is reported by the
    genesis, ie it's GetRoundNumber method. At this time our tooling always generates root
    genesis for round 1);
*/
func NewGenesisStore(dbFile string, seed *genesis.RootGenesis) (*genesisStore, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt DB: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(rootGenesisBucket)
		if err != nil {
			return fmt.Errorf("creating the %q bucket: %w", rootGenesisBucket, err)
		}
		if k, v := b.Cursor().First(); k == nil && v == nil && seed != nil {
			return saveConfiguration(b, seed.GetRoundNumber(), seed)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initializing the DB: %w", err)
	}

	return &genesisStore{db: db}, nil
}

/*
AddConfiguration adds or overrides genesis (configuration) for the given round.
*/
func (gs *genesisStore) AddConfiguration(round uint64, cfg *genesis.RootGenesis) error {
	return gs.db.Update(func(tx *bolt.Tx) error {
		return saveConfiguration(tx.Bucket(rootGenesisBucket), round, cfg)
	})
}

/*
Returns:
  - the configurations for partitions in effect on round "round";
  - the round number when next configuration change should take effect (after "round"), if
    there is no next config registered max uint64 is returned;
  - error if any - in case of non nil error the other return values are invalid!
*/
func (gs *genesisStore) PartitionRecords(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
	rg, next, err := gs.activeGenesis(round)
	if err != nil {
		return nil, 0, err
	}
	return rg.Partitions, next, nil
}

/*
Returns:
  - the "genesis configuration" in effect on round "round";
  - the round number when next configuration change should take effect (after "round"), if
    there is no next config registered max uint64 is returned;
  - error if any - in case of non nil error the other return values are invalid!
*/
func (gs *genesisStore) activeGenesis(round uint64) (rg *genesis.RootGenesis, next uint64, _ error) {
	return rg, next, gs.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(rootGenesisBucket).Cursor()
		k, v := c.Seek(roundToKey(round))
		if k == nil {
			// no exact key and no next key - last value must be current active conf
			if k, v = c.Last(); k == nil {
				// db is empty? shouldn't really happen, something is very wrong!
				return fmt.Errorf("no configuration for round %d, empty DB", round)
			}
			next = math.MaxUint64
		} else {
			if kr := keyToRound(k); kr > round {
				// not exact match, we're on the next one...
				next = keyToRound(k)
				//...so previous record must be the one active for the "round"
				if k, v = c.Prev(); k == nil {
					// if k is nil then there is no prev item - should not happen as the initial genesis
					// is for round 1 (ie this node does not have full genesis history?)
					return fmt.Errorf("no configuration for round %d, missing initial genesis", round)
				}
			} else {
				// must have landed on the exact key is there next conf?
				if k, _ = c.Next(); k != nil {
					next = keyToRound(k)
				} else {
					next = math.MaxUint64
				}
			}
		}

		if err := types.Cbor.Unmarshal(v, &rg); err != nil {
			return fmt.Errorf("decoding root genesis: %w", err)
		}
		return nil
	})
}

func saveConfiguration(bucket *bolt.Bucket, round uint64, cfg *genesis.RootGenesis) error {
	// Verify also validates.
	// But should this be callers responsibility, ie this is just "dumb store"?
	// Ie we do not check does the new conf extend the previous one...
	if err := cfg.Verify(); err != nil {
		return fmt.Errorf("verifying configuration: %w", err)
	}

	b, err := types.Cbor.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing configuration: %w", err)
	}
	if err = bucket.Put(roundToKey(round), b); err != nil {
		return fmt.Errorf("bolt DB write: %w", err)
	}
	return nil
}

func roundToKey(round uint64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, round)
	return key
}

func keyToRound(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
}
