package backend

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type BlockProcessor struct {
	store BillStore
}

func NewBlockProcessor(store BillStore) *BlockProcessor {
	return &BlockProcessor{store: store}
}

func (p *BlockProcessor) ProcessBlock(b *block.Block) error {
	wlog.Info("processing block: ", b.BlockNumber)
	lastBlockNumber, err := p.store.GetBlockNumber()
	if err != nil {
		return err
	}
	if b.BlockNumber != lastBlockNumber+1 {
		return errors.New(fmt.Sprintf("Invalid block height. Received blockNumber %d current wallet blockNumber %d", b.BlockNumber, lastBlockNumber))
	}
	keys, err := p.store.GetKeys()
	if err != nil {
		return err
	}
	for i, tx := range b.Transactions {
		for _, key := range keys {
			err := p.processTx(tx, b, i, key)
			if err != nil {
				return err
			}
		}
	}
	return p.store.SetBlockNumber(b.BlockNumber)
}

func (p *BlockProcessor) processTx(txPb *txsystem.Transaction, b *block.Block, txIdx int, pubKey *Pubkey) error {
	gtx, err := moneytx.NewMoneyTx(alphabillMoneySystemId, txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case moneytx.Transfer:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.NewBearer()) {
			wlog.Info("received transfer order")
			err = p.saveBillWithProof(pubKey.Pubkey, b, &Bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case moneytx.TransferDC:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.TargetBearer()) {
			wlog.Info("received TransferDC order")
			err = p.saveBillWithProof(pubKey.Pubkey, b, &Bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case moneytx.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := p.store.ContainsBill(pubKey.Pubkey, tx.UnitID())
		if err != nil {
			return err
		}
		if containsBill {
			err = p.saveBillWithProof(pubKey.Pubkey, b, &Bill{
				Id:    tx.UnitID(),
				Value: tx.RemainingValue(),
			})
			if err != nil {
				return err
			}
		}
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.TargetBearer()) {
			id := utiltx.SameShardId(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
			err = p.saveBillWithProof(pubKey.Pubkey, b, &Bill{
				Id:    id,
				Value: tx.Amount(),
			})
			if err != nil {
				return err
			}
		}
	case moneytx.Swap:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.OwnerCondition()) {
			err = p.saveBillWithProof(pubKey.Pubkey, b, &Bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
			for _, dustTransfer := range tx.DCTransfers() {
				err := p.store.RemoveBill(pubKey.Pubkey, dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, tx.UnitID())
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

func (p *BlockProcessor) saveBillWithProof(pubkey []byte, b *block.Block, bi *Bill) error {
	genericBlock, err := b.ToGenericBlock(txConverter)
	if err != nil {
		return err
	}
	blockProof, err := block.NewPrimaryProof(genericBlock, bi.Id, crypto.SHA256)
	if err != nil {
		return err
	}
	proof := &BlockProof{
		BillId:      bi.Id,
		BlockNumber: b.BlockNumber,
		BlockProof:  blockProof,
	}
	return p.store.AddBillWithProof(pubkey, bi, proof)
}
