package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/types"
)

// feeCreditTxRecorder container struct for recording fee credit transactions
type feeCreditTxRecorder struct {
	sdrs  map[types.SystemID]*types.SystemDescriptionRecord
	state *state.State
	// recorded fee credit transfers indexed by system_identifier
	transferFeeCredits map[types.SystemID][]*transferFeeCreditTx
	// recorded reclaim fee credit transfers indexed by system_identifier
	reclaimFeeCredits map[types.SystemID][]*reclaimFeeCreditTx
	systemIdentifier  types.SystemID
}

type transferFeeCreditTx struct {
	tx   *types.TransactionOrder
	fee  uint64
	attr *transactions.TransferFeeCreditAttributes
}

type reclaimFeeCreditTx struct {
	tx                  *types.TransactionOrder
	attr                *transactions.ReclaimFeeCreditAttributes
	closeFCTransferAttr *transactions.CloseFeeCreditAttributes
	reclaimFee          uint64
	closeFee            uint64
}

func newFeeCreditTxRecorder(s *state.State, systemIdentifier types.SystemID, records []*types.SystemDescriptionRecord) *feeCreditTxRecorder {
	sdrs := make(map[types.SystemID]*types.SystemDescriptionRecord)
	for _, record := range records {
		sdrs[record.SystemIdentifier] = record
	}
	return &feeCreditTxRecorder{
		sdrs:               sdrs,
		state:              s,
		systemIdentifier:   systemIdentifier,
		transferFeeCredits: make(map[types.SystemID][]*transferFeeCreditTx),
		reclaimFeeCredits:  make(map[types.SystemID][]*reclaimFeeCreditTx),
	}
}

func (f *feeCreditTxRecorder) recordTransferFC(tx *transferFeeCreditTx) {
	sid := tx.attr.TargetSystemIdentifier
	f.transferFeeCredits[sid] = append(f.transferFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) recordReclaimFC(tx *reclaimFeeCreditTx) {
	sid := tx.attr.CloseFeeCreditTransfer.TransactionOrder.SystemID()
	f.reclaimFeeCredits[sid] = append(f.reclaimFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) getAddedCredit(sid types.SystemID) uint64 {
	var sum uint64
	for _, transferFC := range f.transferFeeCredits[sid] {
		sum += transferFC.attr.Amount - transferFC.fee
	}
	return sum
}

func (f *feeCreditTxRecorder) getReclaimedCredit(sid types.SystemID) uint64 {
	var sum uint64
	for _, reclaimFC := range f.reclaimFeeCredits[sid] {
		sum += reclaimFC.closeFCTransferAttr.Amount - reclaimFC.closeFee
	}
	return sum
}

func (f *feeCreditTxRecorder) getSpentFeeSum() uint64 {
	var sum uint64
	for _, transferFCs := range f.transferFeeCredits {
		for _, transferFC := range transferFCs {
			sum += transferFC.fee
		}
	}
	for _, reclaimFCs := range f.reclaimFeeCredits {
		for _, reclaimFC := range reclaimFCs {
			sum += reclaimFC.reclaimFee
		}
	}
	return sum
}

func (f *feeCreditTxRecorder) reset() {
	clear(f.transferFeeCredits)
	clear(f.reclaimFeeCredits)
}

func (f *feeCreditTxRecorder) consolidateFees() error {
	// update fee credit bills for all known partitions with added and removed credits
	for sid, sdr := range f.sdrs {
		addedCredit := f.getAddedCredit(sid)
		reclaimedCredit := f.getReclaimedCredit(sid)
		if addedCredit == reclaimedCredit {
			continue // no update if bill value doesn't change
		}
		fcUnitID := types.UnitID(sdr.FeeCreditBill.UnitID)
		_, err := f.state.GetUnit(fcUnitID, false)
		if err != nil {
			return err
		}
		updateData := state.UpdateUnitData(fcUnitID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", fcUnitID)
				}
				bd.V = bd.V + addedCredit - reclaimedCredit
				return bd, nil
			})
		err = f.state.Apply(updateData)
		if err != nil {
			return fmt.Errorf("failed to update [%x] partition's fee credit bill: %w", sdr.SystemIdentifier, err)
		}

		err = f.state.AddUnitLog(fcUnitID, make([]byte, f.state.HashAlgorithm().Size()))
		if err != nil {
			return fmt.Errorf("failed to update [%x] partition's fee credit bill state log: %w", sdr.SystemIdentifier, err)
		}
	}

	// increment money fee credit bill with spent fees
	spentFeeSum := f.getSpentFeeSum()
	if spentFeeSum > 0 {
		moneyFCUnitID := f.sdrs[f.systemIdentifier].FeeCreditBill.UnitID
		_, err := f.state.GetUnit(moneyFCUnitID, false)
		if err != nil {
			return fmt.Errorf("could not find money fee credit bill: %w", err)
		}
		updateData := state.UpdateUnitData(moneyFCUnitID,
			func(data state.UnitData) (state.UnitData, error) {
				bd, ok := data.(*BillData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain bill data", moneyFCUnitID)
				}
				bd.V = bd.V + spentFeeSum
				return bd, nil
			})
		err = f.state.Apply(updateData)
		if err != nil {
			return fmt.Errorf("failed to update money fee credit bill with spent fees: %w", err)
		}

		err = f.state.AddUnitLog(moneyFCUnitID, make([]byte, f.state.HashAlgorithm().Size()))
		if err != nil {
			return fmt.Errorf("failed to update money fee credit bill state log: %w", err)
		}
	}
	return nil
}
