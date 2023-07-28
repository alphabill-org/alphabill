package money

import (
	"fmt"

	abHasher "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	defaultDustBillDeletionTimeout uint64 = 65536
	PayloadTypeDeleteDustBills            = "deleteDustBills"
)

var (
	// The ID of the dust collector money supply
	dustCollectorMoneySupplyID types.UnitID = make([]byte, 32)

	// Dust collector predicate
	dustCollectorPredicate = script.PredicatePayToPublicKeyHash(script.HashAlgSha256, abHasher.Sum256([]byte("dust collector")), script.SigSchemeSecp256k1)
)

type DustCollector struct {
	// Contains the bill identifiers transferred to the dust collector. The key of the map is the block number when the
	// bill is deleted and its value is transferred to the dust collector.
	dustCollectorBills map[uint64][]types.UnitID
	state              *state.State
	systemIdentifier   []byte
}

func NewDustCollector(s *state.State, systemIdentifier []byte) *DustCollector {
	return &DustCollector{
		state:              s,
		dustCollectorBills: map[uint64][]types.UnitID{},
		systemIdentifier:   systemIdentifier,
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

func (d *DustCollector) handleDust() txsystem.ExecuteFunc {
	return func(tx *types.TransactionOrder, currentBlockNr uint64) (*types.ServerMetadata, error) {
		return &types.ServerMetadata{ActualFee: 0}, d.consolidateDust(currentBlockNr)
	}
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

func (d *DustCollector) generateDcTx(blockNumber uint64) ([]*types.TransactionRecord, error) {
	if len(d.GetDustBills(blockNumber)) == 0 {
		return nil, nil
	}

	return []*types.TransactionRecord{
		{
			TransactionOrder: &types.TransactionOrder{
				Payload: &types.Payload{
					SystemID:       d.systemIdentifier,
					Type:           PayloadTypeDeleteDustBills,
					ClientMetadata: &types.ClientMetadata{Timeout: blockNumber + 1},
				},
			},
			ServerMetadata: &types.ServerMetadata{ActualFee: 0},
		},
	}, nil
}
