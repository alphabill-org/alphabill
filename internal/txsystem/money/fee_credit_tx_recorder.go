package money

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/holiman/uint256"
)

// feeCreditTxRecorder container struct for recording fee credit transactions
type feeCreditTxRecorder struct {
	sdrs  map[string]*genesis.SystemDescriptionRecord
	state *rma.Tree
	// recorded fee credit transfers indexed by string(system_identifier)
	transferFeeCredits map[string][]*transactions.TransferFeeCreditWrapper
	// recorded reclaim fee credit transfers indexed by string(system_identifier)
	reclaimFeeCredits map[string][]*transactions.ReclaimFeeCreditWrapper
	systemIdentifier  string
}

func newFeeCreditTxRecorder(state *rma.Tree, systemIdentifier []byte, records []*genesis.SystemDescriptionRecord) *feeCreditTxRecorder {
	sdrs := make(map[string]*genesis.SystemDescriptionRecord)
	for _, record := range records {
		sdrs[string(record.SystemIdentifier)] = record
	}
	return &feeCreditTxRecorder{
		sdrs:               sdrs,
		state:              state,
		systemIdentifier:   string(systemIdentifier),
		transferFeeCredits: make(map[string][]*transactions.TransferFeeCreditWrapper),
		reclaimFeeCredits:  make(map[string][]*transactions.ReclaimFeeCreditWrapper),
	}
}

func (f *feeCreditTxRecorder) recordTransferFC(tx *transactions.TransferFeeCreditWrapper) {
	sid := string(tx.TransferFC.TargetSystemIdentifier)
	f.transferFeeCredits[sid] = append(f.transferFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) recordReclaimFC(tx *transactions.ReclaimFeeCreditWrapper) {
	sid := string(tx.CloseFCTransfer.SystemID())
	f.reclaimFeeCredits[sid] = append(f.reclaimFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) getAddedCredit(sid string) uint64 {
	var sum uint64
	for _, transferFC := range f.transferFeeCredits[sid] {
		sum += transferFC.TransferFC.Amount
	}
	return sum
}

func (f *feeCreditTxRecorder) getReclaimedCredit(sid string) uint64 {
	var sum uint64
	for _, reclaimFC := range f.reclaimFeeCredits[sid] {
		sum += reclaimFC.CloseFCTransfer.CloseFC.Amount - reclaimFC.CloseFCTransfer.Transaction.ServerMetadata.Fee
	}
	return sum
}

func (f *feeCreditTxRecorder) getSpentFeeSum() uint64 {
	var sum uint64
	for _, transferFCs := range f.transferFeeCredits {
		for _, transferFC := range transferFCs {
			sum += transferFC.Transaction.ServerMetadata.Fee
		}
	}
	for _, reclaimFCs := range f.reclaimFeeCredits {
		for _, reclaimFC := range reclaimFCs {
			sum += reclaimFC.Transaction.ServerMetadata.Fee
		}
	}
	return sum
}

func (f *feeCreditTxRecorder) reset() {
	f.transferFeeCredits = make(map[string][]*transactions.TransferFeeCreditWrapper)
	f.reclaimFeeCredits = make(map[string][]*transactions.ReclaimFeeCreditWrapper)
}

func (f *feeCreditTxRecorder) consolidateFees() error {
	// update fee credit bills for all known partitions with added and removed credits
	for sid, sdr := range f.sdrs {
		addedCredit := f.getAddedCredit(sid)
		reclaimedCredit := f.getReclaimedCredit(sid)
		if addedCredit == reclaimedCredit {
			continue // no update if bill value doesn't change
		}
		fcUnitID := uint256.NewInt(0).SetBytes(sdr.FeeCreditBill.UnitId)
		fcUnit, err := f.state.GetUnit(fcUnitID)
		if err != nil {
			return err
		}
		updateData := rma.UpdateData(fcUnitID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					// TODO updateData should return error
					return data
				}
				bd.V = bd.V + addedCredit - reclaimedCredit
				return bd
			},
			fcUnit.StateHash)
		err = f.state.AtomicUpdate(updateData)
		if err != nil {
			return fmt.Errorf("failed to update [%x] partiton's fee credit bill: %w", sdr.SystemIdentifier, err)
		}
	}

	// increment money fee credit bill with spent fees
	spentFeeSum := f.getSpentFeeSum()
	if spentFeeSum > 0 {
		moneyFCUnitID := uint256.NewInt(0).SetBytes(f.sdrs[string(f.systemIdentifier)].FeeCreditBill.UnitId)
		moneyFCUnit, err := f.state.GetUnit(moneyFCUnitID)
		if err != nil {
			return fmt.Errorf("could not find money fee credit bill: %w", err)
		}
		updateData := rma.UpdateData(moneyFCUnitID,
			func(data rma.UnitData) (newData rma.UnitData) {
				bd, ok := data.(*BillData)
				if !ok {
					// TODO updateData should return error
					return data
				}
				bd.V = bd.V + spentFeeSum
				return bd
			},
			moneyFCUnit.StateHash)
		err = f.state.AtomicUpdate(updateData)
		if err != nil {
			return fmt.Errorf("failed to update money fee credit bill with spent fees: %w", err)
		}
	}
	return nil
}
