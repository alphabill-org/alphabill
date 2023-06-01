package wallet

import (
	"bytes"
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	fc "github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/blocksync"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type BlockProcessor struct {
	txConverter *TxConverter
	db          *storage
}

func NewBlockProcessor(db *storage) *BlockProcessor {
	return &BlockProcessor{
		txConverter: NewTxConverter(vd.DefaultSystemIdentifier),
		db: db,
	}
}

func NewProofFinder(unitID wallet.UnitID, txHash wallet.TxHash, proofCh chan<- *wallet.Proof) blocksync.BlockProcessorFunc {
	return func(_ context.Context, b *block.Block) error {
		log.Debug("Processing block #", b.GetRoundNumber())

		genericBlock, err := b.ToGenericBlock(NewTxConverter(b.SystemIdentifier))
		if err != nil {
			return fmt.Errorf("failed to convert block to generic block: %w", err)
		}

		for _, gtx := range genericBlock.Transactions {
			tx := gtx.ToProtoBuf()

			if unitID != nil && !bytes.Equal(tx.UnitId, unitID){
				continue
			}
			if txHash != nil {
				gtxHash := gtx.Hash(crypto.SHA256)
				if !bytes.Equal(gtxHash, txHash) {
					continue
				}
			}
			proof, err := createProof(tx.UnitId, tx, genericBlock, crypto.SHA256)
			if err != nil {
				return err
			}

			proofCh <- proof
			return nil
		}

		return nil
	}
}

func NewTxPrinter() blocksync.BlockProcessorFunc {
	return func(_ context.Context, b *block.Block) error {
		log.Debug("Processing block #", b.GetRoundNumber())
		for _, tx := range b.Transactions {
			if vd.TypeURLRegisterDataAttributes == tx.TransactionAttributes.TypeUrl {
				log.Info(fmt.Sprintf("Tx in block #%d, hash: %s", b.GetRoundNumber(), hex.EncodeToString(tx.GetUnitId())))
			}
		}

		return nil
	}
}

func (p *BlockProcessor) ProcessBlock(_ context.Context, b *block.Block) error {
	log.Debug("Processing block #", b.GetRoundNumber())

	lastBlockNumber, err := p.db.GetBlockNumber()
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

	return p.db.SetBlockNumber(b.GetRoundNumber())
}

func (p *BlockProcessor) processTx(inTx *txsystem.Transaction, b *block.Block) error {
	gtx, err := p.txConverter.ConvertTx(inTx)
	if err != nil {
		return err
	}

	switch tx := gtx.(type) {
	case vd.RegisterData:
		return p.updateFCB(inTx, b.GetRoundNumber(), tx.Hash(crypto.SHA256))
	case *fc.AddFeeCreditWrapper:
		fcb, err := p.db.GetFeeCreditBill(inTx.UnitId)
		if err != nil {
			return err
		}
		return p.db.SetFeeCreditBill(&FeeCreditBill{
			Id:            inTx.UnitId,
			Value:         fcb.GetValue() + tx.TransferFC.TransferFC.Amount - tx.Transaction.ServerMetadata.Fee,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		})
	case *fc.CloseFeeCreditWrapper:
		fcb, err := p.db.GetFeeCreditBill(inTx.UnitId)
		if err != nil {
			return err
		}
		return p.db.SetFeeCreditBill(&FeeCreditBill{
			Id:            inTx.UnitId,
			Value:         fcb.GetValue() - tx.CloseFC.Amount,
			TxHash:        tx.Hash(crypto.SHA256),
			FCBlockNumber: b.GetRoundNumber(),
		})
	default:
		log.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
}

func (p *BlockProcessor) updateFCB(tx *txsystem.Transaction, roundNumber uint64, txHash []byte) error {
	fcb, err := p.db.GetFeeCreditBill(tx.ClientMetadata.FeeCreditRecordId)
	if err != nil {
		return err
	}
	if fcb == nil {
		return fmt.Errorf("fee credit bill not found: %X", tx.ClientMetadata.FeeCreditRecordId)
	}
	if fcb.Value < tx.ServerMetadata.Fee {
		return fmt.Errorf("fee credit bill value cannot go negative; value=%d fee=%d", fcb.Value, tx.ServerMetadata.Fee)
	}
	fcb.Value -= tx.ServerMetadata.Fee
	fcb.FCBlockNumber = roundNumber
	fcb.TxHash = txHash
	return p.db.SetFeeCreditBill(fcb)
}

func createProof(unitID []byte, tx *txsystem.Transaction, b *block.GenericBlock, hashAlgorithm crypto.Hash) (*wallet.Proof, error) {
	proof, err := block.NewPrimaryProof(b, unitID, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return newTxProof(tx, proof, b.GetRoundNumber())
}

func newTxProof(tx *txsystem.Transaction, proof *block.BlockProof, blockNumber uint64) (*wallet.Proof, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if proof == nil {
		return nil, errors.New("proof is nil")
	}
	return &wallet.Proof{
		Tx:          tx,
		Proof:       proof,
		BlockNumber: blockNumber,
	}, nil
}
