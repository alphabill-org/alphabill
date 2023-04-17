package money

import (
	abHasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/holiman/uint256"
)

const defaultDustBillDeletionTimeout uint64 = 65536

var (
	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID = uint256.NewInt(0)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)
)

type DustCollector struct {
	// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
	// bill is deleted and its value is transferred to the dust collector.
	dustCollectorBills map[uint64][]*uint256.Int
	state              *rma.Tree
}

func NewDustCollector(state *rma.Tree) *DustCollector {
	return &DustCollector{
		state:              state,
		dustCollectorBills: map[uint64][]*uint256.Int{},
	}
}

func (d *DustCollector) AddDustBill(id *uint256.Int, currentBlockNumber uint64) {
	delBlockNr := currentBlockNumber + defaultDustBillDeletionTimeout
	d.dustCollectorBills[delBlockNr] = append(d.dustCollectorBills[delBlockNr], id)
}

func (d *DustCollector) GetDustBills(blockNumber uint64) []*uint256.Int {
	return d.dustCollectorBills[blockNumber]
}

func (d *DustCollector) RemoveDustBills(blockNumber uint64) {
	delete(d.dustCollectorBills, blockNumber)
}

func (d *DustCollector) consolidateDust(currentBlockNumber uint64) error {
	dustBills := d.GetDustBills(currentBlockNumber)
	var valueToTransfer uint64
	for _, billID := range dustBills {
		u, err := d.state.GetUnit(billID)
		if err != nil {
			return err
		}
		bd, ok := u.Data.(*BillData)
		if !ok {
			// it is safe to ignore the data because it is not a bill
			continue
		}
		valueToTransfer += bd.V
		err = d.state.AtomicUpdate(rma.DeleteItem(billID))
		if err != nil {
			return err
		}
	}
	if valueToTransfer > 0 {
		err := d.state.AtomicUpdate(rma.UpdateData(dustCollectorMoneySupplyID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					return bd
				}
				bd.V += valueToTransfer
				return bd
			}, []byte{}))
		if err != nil {
			return err
		}
	}
	d.RemoveDustBills(currentBlockNumber)
	return nil
}
