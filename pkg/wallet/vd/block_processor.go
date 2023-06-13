package wallet

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	fc "github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type blockProcessor struct {
	store Storage
}

func NewBlockProcessor(store Storage) *blockProcessor {
	return &blockProcessor{store: store}
}

func NewProofFinder(unitID wallet.UnitID, txHash wallet.TxHash, proofCh chan<- *wallet.Proof) blocksync.BlockProcessorFunc {
	return func(_ context.Context, b *types.Block) error {
		log.Debug("Processing block #", b.GetRoundNumber())

		for idx, txr := range b.Transactions {
			txo := txr.TransactionOrder
			if unitID != nil && !bytes.Equal(txo.UnitID(), unitID){
				continue
			}
			if txHash != nil {
				txoHash := txo.Hash(crypto.SHA256)
				if !bytes.Equal(txoHash, txHash) {
					continue
				}
			}

			txProof, _, err := types.NewTxProof(b, idx, crypto.SHA256)
			if err != nil {
				return fmt.Errorf("failed to create tx proof for the block: %w", err)
			}

			proofCh <- &wallet.Proof{TxRecord: txr, TxProof: txProof}
			return nil
		}

		return nil
	}
}

func NewTxPrinter() blocksync.BlockProcessorFunc {
	return func(_ context.Context, b *types.Block) error {
		log.Debug("Processing block #", b.GetRoundNumber())
		for _, tx := range b.Transactions {
			txo := tx.TransactionOrder
			if vd.PayloadTypeRegisterData == txo.PayloadType() {
				log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.GetRoundNumber(), hex.EncodeToString(txo.UnitID())))
			}
		}

		return nil
	}
}

func (p *blockProcessor) ProcessBlock(_ context.Context, b *types.Block) error {
	log.Debug("Processing block #", b.GetRoundNumber())

	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return fmt.Errorf("failed to read current block number: %w", err)
	}
	// block numbers must not be sequential (gaps might appear as empty block are not stored
	// and sent) but must be in ascending order
	if lastBlockNumber >= b.GetRoundNumber() {
		return fmt.Errorf("invalid block, received block %d, current wallet block %d", b.GetRoundNumber(), lastBlockNumber)
	}

	for _, tx := range b.Transactions {
		if err := p.processTx(tx, b); err != nil {
			return fmt.Errorf("failed to process tx: %w", err)
		}
	}

	return p.store.SetBlockNumber(b.GetRoundNumber())
}

func (p *blockProcessor) processTx(txr *types.TransactionRecord, b *types.Block) error {
	txo := txr.TransactionOrder

	switch txo.PayloadType() {
	case vd.PayloadTypeRegisterData:
		return p.updateFCB(txr, b.GetRoundNumber())
	case fc.PayloadTypeAddFeeCredit:
		addFeeCreditAttributes := &fc.AddFeeCreditAttributes{}
		if err := txo.UnmarshalAttributes(addFeeCreditAttributes); err != nil {
			return err
		}
		transferFeeCreditAttributes := &fc.TransferFeeCreditAttributes{}
		if err := addFeeCreditAttributes.FeeCreditTransfer.TransactionOrder.UnmarshalAttributes(transferFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:            txo.UnitID(),
			Value:         fcb.GetValue() + transferFeeCreditAttributes.Amount - txr.ServerMetadata.ActualFee,
			TxHash:        txo.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		})
	case fc.PayloadTypeCloseFeeCredit:
		closeFeeCreditAttributes := &fc.CloseFeeCreditAttributes{}
		if err := txo.UnmarshalAttributes(closeFeeCreditAttributes); err != nil {
			return err
		}
		fcb, err := p.store.GetFeeCreditBill(txo.UnitID())
		if err != nil {
			return err
		}
		return p.store.SetFeeCreditBill(&FeeCreditBill{
			Id:            txo.UnitID(),
			Value:         fcb.GetValue() - closeFeeCreditAttributes.Amount,
			TxHash:        txo.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		})
	default:
		log.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", txo.PayloadType()))
		return nil
	}
}

func (p *blockProcessor) updateFCB(txr *types.TransactionRecord, roundNumber uint64) error {
	txo := txr.TransactionOrder
	fcb, err := p.store.GetFeeCreditBill(txo.Payload.ClientMetadata.FeeCreditRecordID)
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
	fcb.FCBlockNumber = roundNumber
	return p.store.SetFeeCreditBill(fcb)
}
