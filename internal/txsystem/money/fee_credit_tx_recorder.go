package money

import "github.com/alphabill-org/alphabill/internal/txsystem/fc"

// feeCreditTxRecorder container struct for recording fee credit transactions
type feeCreditTxRecorder struct {
	// recorded fee credit transfers indexed by string(system_identifier)
	transferFeeCredits map[string][]*fc.TransferFeeCreditWrapper
	// recorded reclaim fee credit transfers indexed by string(system_identifier)
	reclaimFeeCredits map[string][]*fc.ReclaimFeeCreditWrapper
}

func newFeeCreditTxRecorder() *feeCreditTxRecorder {
	return &feeCreditTxRecorder{
		transferFeeCredits: make(map[string][]*fc.TransferFeeCreditWrapper),
		reclaimFeeCredits:  make(map[string][]*fc.ReclaimFeeCreditWrapper),
	}
}

func (f *feeCreditTxRecorder) recordTransferFC(tx *fc.TransferFeeCreditWrapper) {
	sid := string(tx.TransferFC.TargetSystemIdentifier)
	f.transferFeeCredits[sid] = append(f.transferFeeCredits[sid], tx)
}

func (f *feeCreditTxRecorder) recordReclaimFC(tx *fc.ReclaimFeeCreditWrapper) {
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
