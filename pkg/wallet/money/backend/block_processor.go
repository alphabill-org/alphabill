package backend

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
)

const (
	DustBillDeletionTimeout    = 65536
	ExpiredBillDeletionTimeout = 65536
)

type BlockProcessor struct {
	store    BillStore
	sdrs     map[string]*genesis.SystemDescriptionRecord
	moneySDR *genesis.SystemDescriptionRecord
}

func NewBlockProcessor(store BillStore, moneySystemID []byte) (*BlockProcessor, error) {
	sdrs, err := store.Do().GetSystemDescriptionRecords()
	if err != nil {
		return nil, fmt.Errorf("failed to get system description records: %w", err)
	}
	sdrsMap := map[string]*genesis.SystemDescriptionRecord{}
	for _, sdr := range sdrs {
		sdrsMap[string(sdr.SystemIdentifier)] = sdr
	}
	return &BlockProcessor{store: store, sdrs: sdrsMap, moneySDR: sdrsMap[string(moneySystemID)]}, nil
}

func (p *BlockProcessor) ProcessBlock(_ context.Context, b *types.Block) error {
	roundNumber := b.GetRoundNumber()
	wlog.Info("processing block: ", roundNumber)
	return p.store.WithTransaction(func(dbTx BillStoreTx) error {
		lastBlockNumber, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		if lastBlockNumber >= roundNumber {
			return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", roundNumber, lastBlockNumber)
		}
		for i, tx := range b.Transactions {
			if err := p.processTx(tx, b, i, dbTx); err != nil {
				return fmt.Errorf("failed to process transaction: %w", err)
			}
		}
		if err := dbTx.DeleteExpiredBills(roundNumber); err != nil {
			return fmt.Errorf("failed to delete expired bills: %w", err)
		}
		return dbTx.SetBlockNumber(roundNumber)
	})
}

func (p *BlockProcessor) processTx(txr *types.TransactionRecord, b *types.Block, txIdx int, dbTx BillStoreTx) error {
	txo := txr.TransactionOrder
	txHash := txo.Hash(crypto.SHA256)
	roundNumber := b.GetRoundNumber()
	proof, err := sdk.NewTxProof(txIdx, b, crypto.SHA256)
	if err != nil {
		return err
	}

	switch txo.PayloadType() {
	case moneytx.PayloadTypeTransfer:
		wlog.Info(fmt.Sprintf("received transfer order (UnitID=%x)", txo.UnitID()))
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
		wlog.Info(fmt.Sprintf("received TransferDC order (UnitID=%x)", txo.UnitID()))
		err := p.updateFCB(dbTx, txr)
		if err != nil {
			return err
		}
		attr := &moneytx.TransferDCAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}
		err = dbTx.SetBill(&Bill{
			Id:             txo.UnitID(),
			Value:          attr.TargetValue,
			TxHash:         txHash,
			DcNonce:        attr.Nonce,
			OwnerPredicate: attr.TargetBearer,
		}, proof)
		if err != nil {
			return err
		}
		err = dbTx.SetBillExpirationTime(roundNumber+DustBillDeletionTimeout, txo.UnitID())
		if err != nil {
			return err
		}
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
			wlog.Info(fmt.Sprintf("received split order (existing UnitID=%x)", txo.UnitID()))
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
			wlog.Warning(fmt.Sprintf("received split order where existing unit was not found, ignoring tx (unitID=%x)", txo.UnitID()))
		}

		// new bill
		newID := utiltx.SameShardIDBytes(uint256.NewInt(0).SetBytes(txo.UnitID()), moneytx.HashForIDCalculation(txo.UnitID(), txo.Payload.Attributes, txo.Timeout(), crypto.SHA256))
		wlog.Info(fmt.Sprintf("received split order (new UnitID=%x)", newID))
		err = dbTx.SetBill(&Bill{
			Id:             newID,
			Value:          attr.Amount,
			TxHash:         txHash,
			OwnerPredicate: attr.TargetBearer,
		}, proof)
		if err != nil {
			return err
		}
		if err = saveTx(dbTx, attr.TargetBearer, txo, txHash); err != nil {
			return err
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
		wlog.Info(fmt.Sprintf("received swap order (UnitID=%x)", txo.UnitID()))
		err = dbTx.SetBill(&Bill{
			Id:             txo.UnitID(),
			Value:          attr.TargetValue,
			TxHash:         txHash,
			OwnerPredicate: attr.OwnerCondition,
		}, proof)
		if err != nil {
			return err
		}
		for _, dustTransfer := range attr.DcTransfers {
			err := dbTx.RemoveBill(dustTransfer.TransactionOrder.UnitID())
			if err != nil {
				return err
			}
		}
		err = dbTx.DeleteDCMetadata(txo.UnitID())
		if err != nil {
			return err
		}
	case transactions.PayloadTypeTransferFeeCredit:
		wlog.Info(fmt.Sprintf("received transferFC order (UnitID=%x), hash: '%X'", txo.UnitID(), txHash))
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return fmt.Errorf("failed to get bill: %w", err)
		}
		if bill == nil {
			return fmt.Errorf("unit not found for transferFC tx (unitID=%X)", txo.UnitID())
		}
		attr := &transactions.TransferFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return fmt.Errorf("failed to unmarshal transferFC attributes: %w", err)
		}

		v := attr.Amount + txr.ServerMetadata.ActualFee
		if v < bill.Value {
			bill.Value -= v
		} else {
			bill.Value = 0
			// mark bill to be deleted far in the future (approx 1 day)
			// so that bill (proof) can be used for confirmation and follow-up transaction
			// alternatively we can save proofs separately from bills and this hack can be removed
			err = dbTx.SetBillExpirationTime(roundNumber+ExpiredBillDeletionTimeout, txo.UnitID())
			if err != nil {
				return fmt.Errorf("failed to set bill expiration time: %w", err)
			}
		}
		bill.TxHash = txHash
		err = dbTx.SetBill(bill, proof)
		if err != nil {
			return fmt.Errorf("failed to save transferFC bill with proof: %w", err)
		}
		err = p.addTransferredCreditToPartitionFeeBill(dbTx, attr, proof)
		if err != nil {
			return fmt.Errorf("failed to add transferred fee credit to partition fee bill: %w", err)
		}
		err = p.addTxFeeToMoneyFeeBill(dbTx, txr, proof)
		if err != nil {
			return fmt.Errorf("failed to add tx fees to money fee bill: %w", err)
		}
		return nil
	case transactions.PayloadTypeAddFeeCredit:
		wlog.Info(fmt.Sprintf("received addFC order (UnitID=%x), hash: '%X'", txo.UnitID(), txHash))
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
		txHash := txo.Hash(crypto.SHA256)
		return dbTx.SetFeeCreditBill(&Bill{
			Id:          txo.UnitID(),
			Value:       fcb.getValue() + transferFCAttr.Amount - txr.ServerMetadata.ActualFee,
			TxHash:      txHash,
			AddFCTxHash: txHash,
		}, proof)
	case transactions.PayloadTypeCloseFeeCredit:
		wlog.Info(fmt.Sprintf("received closeFC order (UnitID=%x)", txo.UnitID()))
		fcb, err := dbTx.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return err
		}
		attr := &transactions.CloseFeeCreditAttributes{}
		err = txo.UnmarshalAttributes(attr)
		if err != nil {
			return err
		}
		return dbTx.SetFeeCreditBill(&Bill{
			Id:          txo.UnitID(),
			TxHash:      txHash,
			Value:       fcb.getValue() - attr.Amount,
			AddFCTxHash: fcb.getAddFCTxHash(),
		}, proof)
	case transactions.PayloadTypeReclaimFeeCredit:
		wlog.Info(fmt.Sprintf("received reclaimFC order (UnitID=%x)", txo.UnitID()))
		bill, err := dbTx.GetBill(txo.UnitID())
		if err != nil {
			return err
		}
		if bill == nil {
			return fmt.Errorf("unit not found for reclaimFC tx (unitID=%X)", txo.UnitID())
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
	default:
		wlog.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", txo.PayloadType()))
		return nil
	}
	return nil
}

func saveTx(dbTx BillStoreTx, bearer sdk.Predicate, txo *types.TransactionOrder, txHash sdk.TxHash) error {
	isP2pkh, bearer := extractOwnerFromP2pkh(bearer)
	if isP2pkh {
		_, sender := extractOwnerFromProof(txo.OwnerProof)
		hash := sdk.PubKey(bearer).Hash()
		if err := dbTx.StoreTxHistoryRecord(hash, &sdk.TxHistoryRecord{
			UnitID:       txo.UnitID(),
			TxHash:       txHash,
			Timeout:      txo.Timeout(),
			State:        sdk.CONFIRMED,
			Kind:         sdk.INCOMING,
			CounterParty: sender,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *BlockProcessor) addTransferredCreditToPartitionFeeBill(dbTx BillStoreTx, tx *transactions.TransferFeeCreditAttributes, proof *sdk.Proof) error {
	sdr, f := p.sdrs[string(tx.TargetSystemIdentifier)]
	if !f {
		return fmt.Errorf("received transferFC for unknown tx system: %x", tx.TargetSystemIdentifier)
	}
	partitionFeeBill, err := dbTx.GetBill(sdr.FeeCreditBill.UnitId)
	if err != nil {
		return err
	}
	if partitionFeeBill == nil {
		return fmt.Errorf("partition fee bill not found: %x", sdr.FeeCreditBill.UnitId)
	}
	partitionFeeBill.Value += tx.Amount
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
