package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/api/types"
	abHasher "github.com/alphabill-org/alphabill/common/hash"
	"github.com/alphabill-org/alphabill/txsystem/state"
)

const defaultDustBillDeletionTimeout uint64 = 65536

var (
	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = NewBillID(nil, nil)

	// Dust collector predicate
	dustCollectorPredicate = templates.NewP2pkh256BytesFromKeyHash(abHasher.Sum256([]byte("dust collector")))
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

func (d *DustCollector) consolidateDust(currentBlockNumber uint64) error {
	dustBills := d.GetDustBills(currentBlockNumber)
	var valueToTransfer uint64
	for _, billID := range dustBills {
		u, err := d.state.GetUnit(billID, false)
		if err != nil {
			return err
		}
		bd, ok := u.Data().(*BillData)
		if !ok {
			// it is safe to ignore the data because it is not a bill
			continue
		}
		valueToTransfer += bd.V
		err = d.state.Apply(state.DeleteUnit(billID))
		if err != nil {
			return err
		}
	}
	if valueToTransfer > 0 {
		err := d.state.Apply(state.UpdateUnitData(dustCollectorMoneySupplyID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", dustCollectorMoneySupplyID)
				}
				bd.V += valueToTransfer
				return bd, nil
			}))
		if err != nil {
			return err
		}
	}
	d.RemoveDustBills(currentBlockNumber)
	return nil
}
