package genesis

import (
	"errors"
	"math"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

/*
NewGenesisStore creates in-memory fixed genesis store (ie the store
always returns the same genesis info, for all rounds).
*/
func NewGenesisStore(seed *genesis.RootGenesis) *RootGenesisStore {
	return &RootGenesisStore{cfg: seed}
}

/*
NewGenesisStoreFromPartitions wraps the "seed" into RootGenesis struct and calls [NewGenesisStore].
Note that the GenesisRootRecord in the RootGenesis will be unassigned and thus the returned store
should be only used when that field/data is not needed/accessed!
*/
func NewGenesisStoreFromPartitions(seed []*genesis.GenesisPartitionRecord) *RootGenesisStore {
	return NewGenesisStore(&genesis.RootGenesis{Version: 1, Partitions: seed})
}

// in-memory fixed genesis store (ie always returns the same genesis info)
type RootGenesisStore struct {
	cfg *genesis.RootGenesis
}

func (gs *RootGenesisStore) AddConfiguration(round uint64, cfg *genesis.RootGenesis) error {
	// just to keep it simple - tests do not need dynamic configuration management...
	return errors.New("this genesis store does not support adding configurations")
}

func (gs *RootGenesisStore) PartitionRecords(round uint64) ([]*genesis.GenesisPartitionRecord, uint64, error) {
	return gs.cfg.Partitions, math.MaxUint64, nil
}
