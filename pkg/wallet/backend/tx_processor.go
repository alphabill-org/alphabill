package backend

import (
	"bytes"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

type txProcessor struct {
	store BillStore
}

func newTxProcessor(store BillStore) *txProcessor {
	return &txProcessor{store: store}
}

func (p *txProcessor) processTx(txPb *txsystem.Transaction, b *block.Block, txIdx int, pubKey *pubkey) error {
	gtx, err := moneytx.NewMoneyTx(alphabillMoneySystemId, txPb)
	if err != nil {
		return err
	}
	stx := gtx.(txsystem.GenericTransaction)

	switch tx := stx.(type) {
	case moneytx.Transfer:
		isOwner, err := verifyOwner(pubKey.pubkeyHash, tx.NewBearer())
		if err != nil {
			return err
		}
		if isOwner {
			log.Info("received transfer order")
			err = p.saveBillWithProof(pubKey.pubkey, b, txIdx, &bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.pubkey, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case moneytx.TransferDC:
		isOwner, err := verifyOwner(pubKey.pubkeyHash, tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			log.Info("received TransferDC order")
			err = p.saveBillWithProof(pubKey.pubkey, b, txIdx, &bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
		} else {
			err := p.store.RemoveBill(pubKey.pubkey, tx.UnitID())
			if err != nil {
				return err
			}
		}
	case moneytx.Split:
		// split tx contains two bills: existing bill and new bill
		// if any of these bills belong to wallet then we have to
		// 1) update the existing bill and
		// 2) add the new bill
		containsBill, err := p.store.ContainsBill(pubKey.pubkey, tx.UnitID())
		if err != nil {
			return err
		}
		if containsBill {
			err = p.saveBillWithProof(pubKey.pubkey, b, txIdx, &bill{
				Id:    tx.UnitID(),
				Value: tx.RemainingValue(),
			})
			if err != nil {
				return err
			}
		}
		isOwner, err := verifyOwner(pubKey.pubkeyHash, tx.TargetBearer())
		if err != nil {
			return err
		}
		if isOwner {
			id := utiltx.SameShardId(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
			err = p.saveBillWithProof(pubKey.pubkey, b, txIdx, &bill{
				Id:    id,
				Value: tx.Amount(),
			})
			if err != nil {
				return err
			}
		}
	case moneytx.Swap:
		isOwner, err := verifyOwner(pubKey.pubkeyHash, tx.OwnerCondition())
		if err != nil {
			return err
		}
		if isOwner {
			err = p.saveBillWithProof(pubKey.pubkey, b, txIdx, &bill{
				Id:    tx.UnitID(),
				Value: tx.TargetValue(),
			})
			if err != nil {
				return err
			}
			for _, dustTransfer := range tx.DCTransfers() {
				err := p.store.RemoveBill(pubKey.pubkey, dustTransfer.UnitID())
				if err != nil {
					return err
				}
			}
		} else {
			err := p.store.RemoveBill(pubKey.pubkey, tx.UnitID())
			if err != nil {
				return err
			}
		}
	default:
		log.Warning(fmt.Sprintf("received unknown transaction type, skipping processing: %s", tx))
		return nil
	}
	return nil
}

func (p *txProcessor) saveBillWithProof(pubkey []byte, b *block.Block, txIdx int, bi *bill) error {
	err := p.store.AddBill(pubkey, bi)
	if err != nil {
		return err
	}
	bp, err := wallet.ExtractBlockProof(b, txIdx, crypto.SHA256)
	if err != nil {
		return err
	}
	billIdBytes := bi.Id.Bytes32()
	return p.store.SetBlockProof(&blockProof{
		BillId:      billIdBytes[:],
		BlockNumber: b.BlockNumber,
		BlockProof:  bp,
	})
}

// verifyOwner checks if given p2pkh bearer predicate contains given pubKey hash
func verifyOwner(pubkeyHashes *wallet.KeyHashes, bp []byte) (bool, error) {
	// p2pkh predicate: [0x53, 0x76, 0xa8, 0x01, 0x4f, 0x01, <32 bytes>, 0x87, 0x69, 0xac, 0x01]
	// p2pkh predicate: [Dup, Hash <SHA256>, PushHash <SHA256> <32 bytes>, Equal, Verify, CheckSig <secp256k1>]

	// p2pkh owner predicate must be 10 + (32 or 64) (SHA256 or SHA512) bytes long
	if len(bp) != 42 && len(bp) != 74 {
		return false, nil
	}
	// 5th byte is PushHash 0x4f
	if bp[4] != 0x4f {
		return false, nil
	}
	// 6th byte is HashAlgo 0x01 or 0x02 for SHA256 and SHA512 respectively
	hashAlgo := bp[5]
	if hashAlgo == 0x01 {
		return bytes.Equal(bp[6:38], pubkeyHashes.Sha256), nil
	} else if hashAlgo == 0x02 {
		return bytes.Equal(bp[6:70], pubkeyHashes.Sha512), nil
	}
	return false, nil
}
