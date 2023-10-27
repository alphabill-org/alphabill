package backend

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

const (
	DustBillDeletionTimeout    = 65536
	ExpiredBillDeletionTimeout = 65536
)

type BlockProcessor struct {
	store    BillStore
	sdrs     map[string]*genesis.SystemDescriptionRecord
	moneySDR *genesis.SystemDescriptionRecord
	log      *slog.Logger
}

func NewBlockProcessor(store BillStore, moneySystemID []byte, log *slog.Logger) (*BlockProcessor, error) {
	sdrs, err := store.Do().GetSystemDescriptionRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to get system description records: %w", err)
	}
	sdrsMap := map[string]*genesis.SystemDescriptionRecord{}
	for _, sdr := range sdrs {
		sdrsMap[string(sdr.SystemIdentifier)] = sdr
	}
	return &BlockProcessor{store: store, sdrs: sdrsMap, moneySDR: sdrsMap[string(moneySystemID)], log: log}, nil
}

func (p *BlockProcessor) ProcessBlock(ctx context.Context, b *types.Block) error {
	roundNumber := b.GetRoundNumber()
	log := p.log.With(logger.Round(roundNumber))
	log.InfoContext(ctx, fmt.Sprintf("processing block with %d transactions", len(b.Transactions)))
	return p.store.WithTransaction(func(dbTx BillStoreTx) error {
		lastBlockNumber, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		if lastBlockNumber >= roundNumber {
			return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", roundNumber, lastBlockNumber)
		}
		for i, tx := range b.Transactions {
			if err := p.processTx(tx, b, i, dbTx, log); err != nil {
				return fmt.Errorf("failed to process transaction: %w", err)
			}
		}
		if err := dbTx.DeleteExpiredBills(roundNumber); err != nil {
			return fmt.Errorf("failed to delete expired bills: %w", err)
		}
		return dbTx.SetBlockNumber(roundNumber)
	})
}

func (p *BlockProcessor) processTx(txr *types.TransactionRecord, b *types.Block, txIdx int, dbTx BillStoreTx, log *slog.Logger) error {
	txo := txr.TransactionOrder
	txHash := txo.Hash(crypto.SHA256)
	proof, err := sdk.NewTxProof(txIdx, b, crypto.SHA256)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("processing %q order", txo.PayloadType()), logger.UnitID(txo.UnitID()))

	switch txo.PayloadType() {
	case moneytx.PayloadTypeTransfer:
		if err = p.updateFCB(dbTx, txr); err != nil {
			return err
		}
		attr := &moneytx.TransferAttributes{}
		if err = txo.UnmarshalAttributes(attr); err != nil {
			return err
		}
		if err = dbTx.SetBill(&Bill{
			Id:             txo.UnitID(),
			Value:          attr.TargetValue,
			TxHash:         txHash,
			OwnerPredicate: attr.NewBearer,
		}, proof); err != nil {
			return err
		}
		if err = saveTx(dbTx, attr.NewBearer, txo, txHash); err != nil {
			return err
		}
	case moneytx.PayloadTypeTransDC:
		err := p.updateFCB(dbTx, txr)
		if err != nil {
			return err
		}
		attr := &moneytx.TransferDCAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}

		// update bill value, txHash, target unit
		dcBill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return fmt.Errorf("failed to fetch bill: %w", err)
		}
		if dcBill == nil {
			return fmt.Errorf("bill not found: %s", txo.UnitID())
		}
		dcBill.Value = attr.Value
		dcBill.TxHash = txHash
		dcBill.DCTargetUnitID = attr.TargetUnitID
		dcBill.DCTargetUnitBacklink = attr.TargetUnitBacklink
		err = dbTx.SetBill(dcBill, proof)
		if err != nil {
			return err
		}
		// TODO AB-1133
		//err = dbTx.SetBillExpirationTime(roundNumber+DustBillDeletionTimeout, txo.UnitID())
		//if err != nil {
		//	return err
		//}
	case moneytx.PayloadTypeSplit:
		err := p.updateFCB(dbTx, txr)
		if err != nil {
			return err
		}
		attr := &moneytx.SplitAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}
		// old bill
		oldBill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if oldBill != nil {
			err = dbTx.SetBill(&Bill{
				Id:             txo.UnitID(),
				Value:          attr.RemainingValue,
				TxHash:         txHash,
				OwnerPredicate: oldBill.OwnerPredicate,
			}, proof)
			if err != nil {
				return err
			}
		} else {
			// we should always have the "previous bill" other than splitting the initial bill or some error condition
			log.Warn("received split order where existing unit was not found, ignoring tx", logger.UnitID(txo.UnitID()))
		}

		// new bills
		for i, targetUnit := range attr.TargetUnits {
			newID := moneytx.NewBillID(txo.UnitID(), moneytx.HashForIDCalculation(txo.UnitID(), txo.Payload.Attributes, txo.Timeout(), uint32(i), crypto.SHA256))
			log.Info(fmt.Sprintf("new UnitID=%x for split order", newID), logger.UnitID(txo.UnitID()))
			err = dbTx.SetBill(&Bill{
				Id:             newID,
				TxHash:         txHash,
				Value:          targetUnit.Amount,
				OwnerPredicate: targetUnit.OwnerCondition,
			}, proof)
			if err != nil {
				return fmt.Errorf("failed to store split tx unit for new id %s at index %d", newID, i)
			}
			if err := saveTx(dbTx, targetUnit.OwnerCondition, txo, txHash); err != nil {
				return fmt.Errorf("failed to store tx history record: %w", err)
			}
		}
	case moneytx.PayloadTypeSwapDC:
		err := p.updateFCB(dbTx, txr)
		if err != nil {
			return err
		}
		attr := &moneytx.SwapDCAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("existing bill not found for swap tx (UnitID=%s)", txo.UnitID())
		}
		bill.Value += attr.TargetValue
		bill.TxHash = txHash
		bill.OwnerPredicate = attr.OwnerCondition
		bill.Locked = 0
		err = dbTx.SetBill(bill, proof)
		if err != nil {
			return err
		}
		for _, dustTransfer := range attr.DcTransfers {
			err := dbTx.RemoveBill(dustTransfer.TransactionOrder.UnitID())
			if err != nil {
				return err
			}
		}
	case moneytx.PayloadTypeLock:
		if err := p.updateFCB(dbTx, txr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for lock tx (unitID=%s)", txo.UnitID())
		}
		attr := &moneytx.LockAttributes{}
		if err := txo.UnmarshalAttributes(attr); err != nil {
			return err
		}
		bill.Locked = attr.LockStatus
		bill.TxHash = txHash
		return dbTx.SetBill(bill, proof)
	case moneytx.PayloadTypeUnlock:
		if err := p.updateFCB(dbTx, txr); err != nil {
			return fmt.Errorf("failed to update fee credit bill: %w", err)
		}
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for unlock tx (unitID=%s)", txo.UnitID())
		}
		attr := &moneytx.UnlockAttributes{}
		if err := txo.UnmarshalAttributes(attr); err != nil {
			return err
		}
		bill.Locked = 0
		bill.TxHash = txHash
		return dbTx.SetBill(bill, proof)
	case transactions.PayloadTypeTransferFeeCredit:
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return fmt.Errorf("failed to get bill: %w", err)
		}
		if bill == nil {
			return fmt.Errorf("unit not found for transferFC tx (unitID=%s)", txo.UnitID())
		}
		attr := &transactions.TransferFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return fmt.Errorf("failed to unmarshal transferFC attributes: %w", err)
		}
		if attr.Amount < bill.Value {
			bill.Value -= attr.Amount
			bill.TxHash = txHash
			if err := dbTx.SetBill(bill, proof); err != nil {
				return fmt.Errorf("failed to save transferFC bill with proof: %w", err)
			}
		} else {
			if err := dbTx.StoreTxProof(txo.UnitID(), txHash, proof); err != nil {
				return fmt.Errorf("failed to store tx proof zero value bill: %w", err)
			}
			if err := dbTx.RemoveBill(bill.Id); err != nil {
				return fmt.Errorf("failed to remove zero value bill: %w", err)
			}
		}
		err = p.addTransferredCreditToPartitionFeeBill(dbTx, attr, proof, txr.ServerMetadata.ActualFee)
		if err != nil {
			return fmt.Errorf("failed to add transferred fee credit to partition fee bill: %w", err)
		}
		err = p.addTxFeeToMoneyFeeBill(dbTx, txr, proof)
		if err != nil {
			return fmt.Errorf("failed to add tx fees to money fee bill: %w", err)
		}
		err = p.addLockedFeeCredit(dbTx, attr.TargetSystemIdentifier, attr.TargetRecordID, txr)
		if err != nil {
			return fmt.Errorf("failed to add locked fee credit: %w", err)
		}
		return nil
	case transactions.PayloadTypeAddFeeCredit:
		fcb, err := dbTx.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return err
		}
		addFCAttr := &transactions.AddFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(addFCAttr)
		if err != nil {
			return err
		}
		transferFCAttr := &transactions.TransferFeeCreditAttributes{}
		err = addFCAttr.FeeCreditTransfer.TransactionOrder.UnmarshalAttributes(transferFCAttr)
		if err != nil {
			return err
		}
		return dbTx.SetFeeCreditBill(&Bill{
			Id:     txo.UnitID(),
			Value:  fcb.getValue() + transferFCAttr.Amount - addFCAttr.FeeCreditTransfer.ServerMetadata.ActualFee - txr.ServerMetadata.ActualFee,
			TxHash: txHash,
		}, proof)
	case transactions.PayloadTypeCloseFeeCredit:
		fcb, err := dbTx.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return err
		}
		attr := &transactions.CloseFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}
		err = p.addClosedFeeCredit(dbTx, txo.UnitID(), txr)
		if err != nil {
			return err
		}
		return dbTx.SetFeeCreditBill(&Bill{
			Id:     txo.UnitID(),
			TxHash: txHash,
			Value:  fcb.getValue() - attr.Amount,
		}, proof)
	case transactions.PayloadTypeReclaimFeeCredit:
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for reclaimFC tx (unitID=%s)", txo.UnitID())
		}
		reclaimFCAttr := &transactions.ReclaimFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(reclaimFCAttr)
		if err != nil {
			return err
		}
		closeFCTXR := reclaimFCAttr.CloseFeeCreditTransfer
		closeFCTXO := closeFCTXR.TransactionOrder
		closeFCAttr := &transactions.CloseFeeCreditAttributes{}
		err = closeFCTXO.UnmarshalAttributes(closeFCAttr)
		if err != nil {
			return err
		}

		// 1. remove reclaimed amount from user bill
		reclaimedValue := closeFCAttr.Amount - closeFCTXR.ServerMetadata.ActualFee - txr.ServerMetadata.ActualFee
		bill.Value += reclaimedValue
		bill.TxHash = txHash
		bill.Locked = 0
		err = dbTx.SetBill(bill, proof)
		if err != nil {
			return err
		}
		// 2. remove reclaimed amount from partition fee bill
		err = p.removeReclaimedCreditFromPartitionFeeBill(dbTx, closeFCTXR, closeFCAttr, proof)
		if err != nil {
			return err
		}
		// 3. add reclaimFC tx fee to money partition fee bill
		return p.addTxFeeToMoneyFeeBill(dbTx, txr, proof)
	case transactions.PayloadTypeLockFeeCredit:
		fcb, err := dbTx.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return fmt.Errorf("failed to load fee credit bill: %w", err)
		}
		if fcb == nil {
			return fmt.Errorf("fee credit bill not found: %X", txo.Payload.ClientMetadata.FeeCreditRecordID)
		}
		if fcb.Value < txr.ServerMetadata.ActualFee {
			return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, txr.ServerMetadata.ActualFee)
		}
		attr := &transactions.LockFeeCreditAttributes{}
		if err := txo.UnmarshalAttributes(attr); err != nil {
			return fmt.Errorf("failed to unmarshal lockFC attributes: %w", err)
		}
		fcb.Locked = attr.LockStatus
		fcb.Value -= txr.ServerMetadata.ActualFee
		fcb.TxHash = txHash
		return dbTx.SetFeeCreditBill(fcb, proof)
	case transactions.PayloadTypeUnlockFeeCredit:
		fcb, err := dbTx.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return fmt.Errorf("failed to load fee credit bill: %w", err)
		}
		if fcb == nil {
			return fmt.Errorf("fee credit bill not found: %X", txo.Payload.ClientMetadata.FeeCreditRecordID)
		}
		if fcb.Value < txr.ServerMetadata.ActualFee {
			return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, txr.ServerMetadata.ActualFee)
		}
		fcb.Locked = 0
		fcb.Value -= txr.ServerMetadata.ActualFee
		fcb.TxHash = txHash
		return dbTx.SetFeeCreditBill(fcb, proof)
	default:
		log.Warn(fmt.Sprintf("no handler for transaction type %q, skipping processing", txo.PayloadType()), logger.UnitID(txo.UnitID()))
	}
	return nil
}

func saveTx(dbTx BillStoreTx, bearer sdk.Predicate, txo *types.TransactionOrder, txHash sdk.TxHash) error {
	bearerKeyHash := extractOwnerHashFromP2pkh(bearer)
	if bearerKeyHash != nil {
		if err := dbTx.StoreTxHistoryRecord(bearerKeyHash, &sdk.TxHistoryRecord{
			UnitID:       txo.UnitID(),
			TxHash:       txHash,
			Timeout:      txo.Timeout(),
			State:        sdk.CONFIRMED,
			Kind:         sdk.INCOMING,
			CounterParty: extractOwnerKeyFromProof(txo.OwnerProof),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *BlockProcessor) addTransferredCreditToPartitionFeeBill(dbTx BillStoreTx, tx *transactions.TransferFeeCreditAttributes, proof *sdk.Proof, actualFee uint64) error {
	sdr, f := p.sdrs[string(tx.TargetSystemIdentifier)]
	if !f {
		return fmt.Errorf("received transferFC for unknown tx system: %x", tx.TargetSystemIdentifier)
	}
	partitionFeeBill, err := dbTx.GetBill(sdr.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	if partitionFeeBill == nil {
		return fmt.Errorf("partition fee bill not found: systemID=%x unitID=%x", tx.TargetSystemIdentifier, sdr.FeeCreditBill.UnitId)
	}
	partitionFeeBill.Value += tx.Amount - actualFee
	return dbTx.SetBill(partitionFeeBill, proof)
}

func (p *BlockProcessor) removeReclaimedCreditFromPartitionFeeBill(dbTx BillStoreTx, txr *types.TransactionRecord, attr *transactions.CloseFeeCreditAttributes, proof *sdk.Proof) error {
	txo := txr.TransactionOrder
	sdr, f := p.sdrs[string(txo.SystemID())]
	if !f {
		return fmt.Errorf("received reclaimFC for unknown tx system: %x", txo.SystemID())
	}
	partitionFeeBill, err := dbTx.GetBill(sdr.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	if partitionFeeBill == nil {
		return fmt.Errorf("partition fee bill not found: systemID=%x unitID=%x", txo.SystemID(), sdr.FeeCreditBill.UnitId)
	}
	partitionFeeBill.Value -= attr.Amount
	partitionFeeBill.Value += txr.ServerMetadata.ActualFee
	return dbTx.SetBill(partitionFeeBill, proof)
}

func (p *BlockProcessor) addTxFeeToMoneyFeeBill(dbTx BillStoreTx, tx *types.TransactionRecord, proof *sdk.Proof) error {
	moneyFeeBill, err := dbTx.GetBill(p.moneySDR.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	moneyFeeBill.Value += tx.ServerMetadata.ActualFee
	return dbTx.SetBill(moneyFeeBill, proof)
}

func (p *BlockProcessor) updateFCB(dbTx BillStoreTx, txr *types.TransactionRecord) error {
	txo := txr.TransactionOrder
	fcb, err := dbTx.GetFeeCreditBill(txo.Payload.ClientMetadata.FeeCreditRecordID)
	if err != nil {
		return err
	}
	if fcb == nil {
		return fmt.Errorf("fee credit bill not found: %X", txo.Payload.ClientMetadata.FeeCreditRecordID)
	}
	if fcb.Value < txr.ServerMetadata.ActualFee {
		return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, txr.ServerMetadata.ActualFee)
	}
	fcb.Value -= txr.ServerMetadata.ActualFee
	return dbTx.SetFeeCreditBill(fcb, nil)
}

func (p *BlockProcessor) addLockedFeeCredit(dbTx BillStoreTx, systemID, targetRecordID []byte, txr *types.TransactionRecord) error {
	return dbTx.SetLockedFeeCredit(systemID, targetRecordID, txr)
}

func (p *BlockProcessor) addClosedFeeCredit(dbTx BillStoreTx, targetRecordID []byte, txr *types.TransactionRecord) error {
	return dbTx.SetClosedFeeCredit(targetRecordID, txr)
}
