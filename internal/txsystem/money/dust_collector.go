package money

import (
	abHasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
)

const defaultDustBillDeletionTimeout uint64 = 65536

var (
	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = NewBillID(nil, nil)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)
)

type DustCollector struct {
	// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
	// bill is deleted and its value is transferred to the dust collector.
	dustCollectorBills map[uint64][]types.UnitID
	state              *state.State
}

func NewDustCollector(s *state.State) *DustCollector {
	return &DustCollector{
		state:              s,
		dustCollectorBills: map[uint64][]types.UnitID{},
	}
}

func (d *DustCollector) AddDustBill(id types.UnitID, currentBlockNumber uint64) {
	delBlockNr := currentBlockNumber + defaultDustBillDeletionTimeout
	d.dustCollectorBills[delBlockNr] = append(d.dustCollectorBills[delBlockNr], id)
}

func (d *DustCollector) GetDustBills(blockNumber uint64) []types.UnitID {
	return d.dustCollectorBills[blockNumber]
}

func (d *DustCollector) RemoveDustBills(blockNumber uint64) {
	delete(d.dustCollectorBills, blockNumber)
}
