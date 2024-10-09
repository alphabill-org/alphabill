package partitions

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

var rootGenesisBucket = []byte("root-genesis")

/*
genesisStore is persistent storage for root chain configuration files.

Bucket "root-genesis" maps round number (uint64) to root genesis file. The idea
is that starting with given round number the configuration described by the genesis
should be used.
*/
type genesisStore struct {
	db         *bolt.DB
	mu         sync.RWMutex
	currentCfg *genesis.RootGenesis
	lastUpdate uint64
	nextUpdate uint64
	log        *slog.Logger
}

/*
  - dbFile is filename (full path) to the Bolt DB file to use for storage, if the
    file does not exist it will be created;
*/
func NewGenesisStore(dbFile string, genesisCfg *genesis.RootGenesis, log *slog.Logger) (*genesisStore, error) {
	db, err := bolt.Open(dbFile, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening bolt DB: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(rootGenesisBucket)
		if err != nil {
			return fmt.Errorf("creating the %q bucket: %w", rootGenesisBucket, err)
		}
		if k, v := b.Cursor().First(); k == nil && v == nil && genesisCfg != nil {
			return saveConfiguration(b, genesis.RootRound, genesisCfg)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("initializing the DB: %w", err)
	}

	return &genesisStore{
		db:         db,
		lastUpdate: 0,
		nextUpdate: 0, // ensures configuration is loaded on next GetConfiguration call
		log:        log,
	}, nil
}

/*
Returns:
  - the configuration in effect on round "round";
  - the round number when the configuration took effect
  - error if any - in case of non nil error the other return values are invalid!
*/
func (gs *genesisStore) GetConfiguration(round uint64) (*genesis.RootGenesis, uint64, error) {
	gs.mu.RLock()
	if gs.lastUpdate <= round && round < gs.nextUpdate {
		// currentCfg is valid for the round
		defer gs.mu.RUnlock()
		return gs.currentCfg, gs.lastUpdate, nil
	}
	gs.mu.RUnlock()

	// currentCfg was not valid for the round, let's load and cache the correct one
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// double-check to see if someone else already loaded it while we acquired lock
	if gs.lastUpdate <= round && round < gs.nextUpdate {
		return gs.currentCfg, gs.lastUpdate, nil
	}

	cfg, lastUpdate, nextUpdate, err := gs.loadConfiguration(round)
	if err != nil {
		return nil, 0, err
	}

	gs.log.Info(fmt.Sprintf("Loaded configuration for round %d", gs.lastUpdate))

	gs.currentCfg = cfg
	gs.lastUpdate = lastUpdate
	gs.nextUpdate = nextUpdate

	return gs.currentCfg, gs.lastUpdate, nil
}

/*
   AddConfiguration registers new configuration taking effect from round "round".
*/
func (gs *genesisStore) AddConfiguration(cfg *genesis.RootGenesis, round uint64) error {
	gs.log.Info(fmt.Sprintf("Adding configuration for round %d", round))

	err := gs.db.Update(func(tx *bolt.Tx) error {
		return saveConfiguration(tx.Bucket(rootGenesisBucket), round, cfg)
	})
	if err != nil {
		return fmt.Errorf("storing configurations for round %d: %w", round, err)
	}

	// Just in case we need to change nextUpdate
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.lastUpdate < round && round < gs.nextUpdate {
		// This configuration potentially (we don't know the current round)
		// invalidates the currently cached one. In any case, set nextUpdate to
		// the given round number. If it happens to be in the past, the correct
		// configuration is just reloaded.
		gs.nextUpdate = round
	}

	return nil
}

/*
Returns:
  - the root chain configuration in effect on round "round";
  - the round number when the returend configuration took effect
  - the round number when next configuration change should take effect (after "round"), if
    there is no next config registered max uint64 is returned;
  - error if any - in case of non nil error the other return values are invalid!
*/
func (gs *genesisStore) loadConfiguration(round uint64) (rg *genesis.RootGenesis, last, next uint64, _ error) {
	return rg, last, next, gs.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket(rootGenesisBucket).Cursor()
		k, v := c.Seek(roundToKey(round))
		if k == nil {
			// no exact key and no next key - last value must be current active conf
			if k, v = c.Last(); k == nil {
				// db is empty? shouldn't really happen, something is very wrong!
				return fmt.Errorf("no configuration for round %d, empty DB", round)
			}
			last = keyToRound(k)
			next = math.MaxUint64
		} else {
			if kr := keyToRound(k); kr > round {
				// not exact match, we're on the next one...
				next = kr
				//...so previous record must be the one active for the "round"
				if k, v = c.Prev(); k == nil {
					// if k is nil then there is no prev item - should not happen as the initial genesis
					// is for round 1 (ie this node does not have full genesis history?)
					return fmt.Errorf("no configuration for round %d, missing initial genesis", round)
				}
				last = keyToRound(k)
			} else {
				// must have landed on the exact key is there next conf?
				last = keyToRound(k)
				if k, _ = c.Next(); k != nil {
					next = keyToRound(k)
				} else {
					next = math.MaxUint64
				}
			}
		}

		if err := types.Cbor.Unmarshal(v, &rg); err != nil {
			return fmt.Errorf("decoding configuration: %w", err)
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
