package money

import (
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const DustBillDeletionTimeout = 65536

type BlockProcessor struct {
	store       BillStore
	TxConverter block.TxConverter
}

func NewBlockProcessor(store BillStore, txConverter block.TxConverter) *BlockProcessor {
	return &BlockProcessor{store: store, TxConverter: txConverter}
}

func (p *BlockProcessor) ProcessBlock(_ context.Context, b *block.Block) error {
	roundNumber := b.UnicityCertificate.InputRecord.RoundNumber
	wlog.Info("processing block: ", roundNumber)
	return p.store.WithTransaction(func(dbTx BillStoreTx) error {
		lastBlockNumber, err := dbTx.GetBlockNumber()
		if err != nil {
			return err
		}
		if lastBlockNumber >= roundNumber {
			return fmt.Errorf("invalid block number. Received blockNumber %d current wallet blockNumber %d", roundNumber, lastBlockNumber)
		}
		for _, tx := range b.Transactions {
			if err := p.processTx(tx, b, dbTx); err != nil {
				return fmt.Errorf("failed to process transaction: %w", err)
			}
		}
		if err := dbTx.DeleteExpiredBills(roundNumber); err != nil {
			return fmt.Errorf("failed to delete expired bills: %w", err)
		}
		return dbTx.SetBlockNumber(roundNumber)
	})
}

func (p *BlockProcessor) processTx(txPb *txsystem.Transaction, b *block.Block, dbTx BillStoreTx) error {
	gtx, err := p.TxConverter.ConvertTx(txPb)
	if err != nil {
		return err
	}
	switch tx := gtx.(type) {
	case moneytx.Transfer:
		wlog.Info(fmt.Sprintf("received transfer order (UnitID=%x)", txPb.UnitId))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.NewBearer(),
		})
		if err != nil {
			return err
		}
	case moneytx.TransferDC:
		wlog.Info(fmt.Sprintf("received TransferDC order (UnitID=%x)", txPb.UnitId))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			IsDCBill:       true,
			OwnerPredicate: tx.TargetBearer(),
		})
		if err != nil {
			return err
		}
		err = dbTx.SetBillExpirationTime(b.UnicityCertificate.InputRecord.RoundNumber+DustBillDeletionTimeout, txPb.UnitId)
		if err != nil {
			return err
		}
	case moneytx.Split:
		// old bill
		oldBill, err := dbTx.GetBill(txPb.UnitId)
		if err != nil {
			return err
		}
		if oldBill != nil {
			wlog.Info(fmt.Sprintf("received split order (existing UnitID=%x)", txPb.UnitId))
			err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
				Id:             txPb.UnitId,
				Value:          tx.RemainingValue(),
				TxHash:         tx.Hash(crypto.SHA256),
				OwnerPredicate: oldBill.OwnerPredicate,
			})
			if err != nil {
				return err
			}
		} else {
			// we should always have the "previous bill" other than splitting the initial bill or some error condition
			wlog.Warning(fmt.Sprintf("received split order where existing unit was not found, ignoring tx (unitID=%x)", txPb.UnitId))
		}

		// new bill
		newID := utiltx.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
		wlog.Info(fmt.Sprintf("received split order (new UnitID=%x)", newID))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             util.Uint256ToBytes(newID),
			Value:          tx.Amount(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.TargetBearer(),
		})
		if err != nil {
			return err
		}
	case moneytx.Swap:
		wlog.Info(fmt.Sprintf("received swap order (UnitID=%x)", txPb.UnitId))
		err = p.saveBillWithProof(b, txPb, dbTx, &Bill{
			Id:             txPb.UnitId,
			Value:          tx.TargetValue(),
			TxHash:         tx.Hash(crypto.SHA256),
			OwnerPredicate: tx.OwnerCondition(),
		})
		if err != nil {
			return err
		}
		for _, dustTransfer := range tx.DCTransfers() {
			err := dbTx.RemoveBill(util.Uint256ToBytes(dustTransfer.UnitID()))
			if err != nil {
				return err
			}
		}
	default:
		wlog.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
	return nil
}

func (p *BlockProcessor) saveBillWithProof(b *block.Block, tx *txsystem.Transaction, dbTx BillStoreTx, bill *Bill) error {
	genericBlock, err := b.ToGenericBlock(p.TxConverter)
	if err != nil {
		return err
	}
	blockProof, err := block.NewPrimaryProof(genericBlock, bill.Id, crypto.SHA256)
	if err != nil {
		return err
	}
	proof := &TxProof{
		BlockNumber: b.UnicityCertificate.InputRecord.RoundNumber,
		Tx:          tx,
		Proof:       blockProof,
	}
	bill.TxProof = proof
	return dbTx.SetBill(bill)
}
