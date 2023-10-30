package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/api/sdr"
	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/state"
)

// feeCreditTxRecorder container struct for recording fee credit transactions
type feeCreditTxRecorder struct {
	sdrs  map[string]*sdr.SystemDescriptionRecord
	state *state.State
	// recorded fee credit transfers indexed by string(system_identifier)
	transferFeeCredits map[string][]*transferFeeCreditTx
	// recorded reclaim fee credit transfers indexed by string(system_identifier)
	reclaimFeeCredits map[string][]*reclaimFeeCreditTx
	systemIdentifier  string
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

func newFeeCreditTxRecorder(s *state.State, systemIdentifier []byte, records []*sdr.SystemDescriptionRecord) *feeCreditTxRecorder {
	sdrs := make(map[string]*sdr.SystemDescriptionRecord)
	for _, record := range records {
		sdrs[string(record.SystemIdentifier)] = record
	}
	return &feeCreditTxRecorder{
		sdrs:               sdrs,
		state:              s,
		systemIdentifier:   string(systemIdentifier),
		transferFeeCredits: make(map[string][]*transferFeeCreditTx),
		reclaimFeeCredits:  make(map[string][]*reclaimFeeCreditTx),
	}
}

func (f *feeCreditTxRecorder) recordTransferFC(tx *transferFeeCreditTx) {
	sid := string(tx.attr.TargetSystemIdentifier)
	f.transferFeeCredits[sid] = append(f.transferFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) recordReclaimFC(tx *reclaimFeeCreditTx) {
	sid := string(tx.attr.CloseFeeCreditTransfer.TransactionOrder.SystemID())
	f.reclaimFeeCredits[sid] = append(f.reclaimFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) getAddedCredit(sid string) uint64 {
	var sum uint64
	for _, transferFC := range f.transferFeeCredits[sid] {
		sum += transferFC.attr.Amount - transferFC.fee
	}
	return sum
}

func (f *feeCreditTxRecorder) getReclaimedCredit(sid string) uint64 {
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
	f.transferFeeCredits = make(map[string][]*transferFeeCreditTx)
	f.reclaimFeeCredits = make(map[string][]*reclaimFeeCreditTx)
}

func (f *feeCreditTxRecorder) consolidateFees() error {
	// update fee credit bills for all known partitions with added and removed credits
	for sid, sdr := range f.sdrs {
		addedCredit := f.getAddedCredit(sid)
		reclaimedCredit := f.getReclaimedCredit(sid)
		if addedCredit == reclaimedCredit {
			continue // no update if bill value doesn't change
		}
		fcUnitID := types.UnitID(sdr.FeeCreditBill.UnitId)
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
			return fmt.Errorf("failed to update [%x] partiton's fee credit bill: %w", sdr.SystemIdentifier, err)
		}
	}

	// increment money fee credit bill with spent fees
	spentFeeSum := f.getSpentFeeSum()
	if spentFeeSum > 0 {
		moneyFCUnitID := f.sdrs[f.systemIdentifier].FeeCreditBill.UnitId
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
	}
	return nil
}
