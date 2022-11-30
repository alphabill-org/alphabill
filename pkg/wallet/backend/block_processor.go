package backend

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
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
		return fmt.Errorf("invalid block height. Received blockNumber %d current wallet blockNumber %d", b.BlockNumber, lastBlockNumber)
	}
	keys, err := p.store.GetKeys()
	if err != nil {
		return err
	}
	for _, tx := range b.Transactions {
		for _, key := range keys {
			err := p.processTx(tx, b, key)
			if err != nil {
				return err
			}
		}
	}
	return p.store.SetBlockNumber(b.BlockNumber)
}

func (p *BlockProcessor) processTx(txPb *txsystem.Transaction, b *block.Block, pubKey *Pubkey) error {
	gtx, err := moneytx.NewMoneyTx(alphabillMoneySystemId, txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case moneytx.Transfer:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.NewBearer()) {
			wlog.Info(fmt.Sprintf("received transfer order (UnitID=%x) for pubkey=%x", tx.UnitID(), pubKey.Pubkey))
			err = p.saveBillWithProof(pubKey.Pubkey, b, txPb, &Bill{
				Id:     txPb.UnitId,
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, txPb.UnitId)
			if err != nil {
				return err
			}
		}
	case moneytx.TransferDC:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.TargetBearer()) {
			wlog.Info(fmt.Sprintf("received TransferDC order (UnitID=%x) for pubkey=%x", tx.UnitID(), pubKey.Pubkey))
			err = p.saveBillWithProof(pubKey.Pubkey, b, txPb, &Bill{
				Id:       txPb.UnitId,
				Value:    tx.TargetValue(),
				TxHash:   tx.Hash(crypto.SHA256),
				IsDCBill: true,
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, txPb.UnitId)
			if err != nil {
				return err
			}
		}
	case moneytx.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := p.store.ContainsBill(pubKey.Pubkey, txPb.UnitId)
		if err != nil {
			return err
		}
		if containsBill {
			wlog.Info(fmt.Sprintf("received split order (existing UnitID=%x) for pubkey=%x", tx.UnitID(), pubKey.Pubkey))
			err = p.saveBillWithProof(pubKey.Pubkey, b, txPb, &Bill{
				Id:     txPb.UnitId,
				Value:  tx.RemainingValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.TargetBearer()) {
			id := utiltx.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
			wlog.Info(fmt.Sprintf("received split order (new UnitID=%x) for pubkey=%x", id, pubKey.Pubkey))
			err = p.saveBillWithProof(pubKey.Pubkey, b, txPb, &Bill{
				Id:     util.Uint256ToBytes(id),
				Value:  tx.Amount(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
		}
	case moneytx.Swap:
		if wallet.VerifyP2PKHOwner(pubKey.PubkeyHash, tx.OwnerCondition()) {
			wlog.Info(fmt.Sprintf("received swap order (UnitID=%x) for pubkey=%x", tx.UnitID(), pubKey.Pubkey))
			err = p.saveBillWithProof(pubKey.Pubkey, b, txPb, &Bill{
				Id:     txPb.UnitId,
				Value:  tx.TargetValue(),
				TxHash: tx.Hash(crypto.SHA256),
			})
			if err != nil {
				return err
			}
			for _, dustTransfer := range tx.DCTransfers() {
				err := p.store.RemoveBill(pubKey.Pubkey, util.Uint256ToBytes(dustTransfer.UnitID()))
				if err != nil {
					return err
				}
			}
		} else {
			err := p.store.RemoveBill(pubKey.Pubkey, txPb.UnitId)
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

func (p *BlockProcessor) saveBillWithProof(pubkey []byte, b *block.Block, tx *txsystem.Transaction, bi *Bill) error {
	genericBlock, err := b.ToGenericBlock(txConverter)
	if err != nil {
		return err
	}
	blockProof, err := block.NewPrimaryProof(genericBlock, uint256.NewInt(0).SetBytes(bi.Id), crypto.SHA256)
	if err != nil {
		return err
	}
	proof := &TxProof{
		BlockNumber: b.BlockNumber,
		Tx:          tx,
		Proof:       blockProof,
	}
	bi.TxProof = proof
	return p.store.SetBills(pubkey, bi)
}
