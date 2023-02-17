package money

import (
	"crypto"
	"errors"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
)

// txLog helper struct used to track pending/confirmed transactions
type txLog struct {
	txsMap map[string]*Bill
}

func NewTxLog(pendingTxs []*txsystem.Transaction) *txLog {
	txsMap := make(map[string]*Bill, len(pendingTxs))
	for _, tx := range pendingTxs {
		txsMap[tx.String()] = nil
	}
	return &txLog{txsMap: txsMap}
}
func (t *txLog) Contains(tx *txsystem.Transaction) bool {
	_, exists := t.txsMap[tx.String()]
	return exists
}

func (t *txLog) RecordTx(tx *txsystem.Transaction, b *block.Block) error {
	bill, err := t.extractBill(tx, b)
	if err != nil {
		return err
	}
	t.txsMap[tx.String()] = bill
	return nil
}

// extractBill extracts bill with proof from given transaction and block
func (t *txLog) extractBill(txPb *txsystem.Transaction, b *block.Block) (*Bill, error) {
	gtx, err := txConverter.ConvertTx(txPb)
	if err != nil {
		return nil, err
	}
	genericBlock, err := b.ToGenericBlock(txConverter)
	if err != nil {
		return nil, err
	}
	var bill *Bill
	switch tx := gtx.(type) {
	case money.Transfer:
		bill = &Bill{
			Id:     tx.UnitID(),
			Value:  tx.TargetValue(),
			TxHash: tx.Hash(crypto.SHA256),
		}
	case money.Split:
		bill = &Bill{
			Id:     utiltx.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
			Value:  tx.Amount(),
			TxHash: tx.Hash(crypto.SHA256),
		}
	case money.TransferDC:
		return nil, errors.New("recorded dc tx (should not happen as we only send transfer and split txs)")
	case money.Swap:
		return nil, errors.New("recorded swap tx (should not happen as we only send transfer and split txs)")
	default:
		return nil, errors.New("recorded unknown transaction type (should not happen as we only send transfer and split txs)")
	}

	// add proof to bill
	proof, err := block.NewPrimaryProof(genericBlock, bill.GetID(), crypto.SHA256)
	if err != nil {
		return nil, err
	}
	blockProof, err := NewBlockProof(txPb, proof, b.BlockNumber)
	if err != nil {
		return nil, err
	}
	bill.BlockProof = blockProof
	return bill, nil
}

func (t *txLog) IsAllTxsConfirmed() bool {
	for _, v := range t.txsMap {
		if v == nil {
			return false
		}
	}
	return true
}

func (t *txLog) GetAllRecordedBills() []*Bill {
	var bills []*Bill
	for _, v := range t.txsMap {
		if v != nil {
			bills = append(bills, v)
		}
	}
	return bills
}
