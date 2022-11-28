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

func newTxLog(pendingTxs []*txsystem.Transaction) *txLog {
	txsMap := make(map[string]*Bill, len(pendingTxs))
	for _, tx := range pendingTxs {
		txsMap[tx.String()] = nil
	}
	return &txLog{txsMap: txsMap}
}
func (t *txLog) contains(tx *txsystem.Transaction) bool {
	_, exists := t.txsMap[tx.String()]
	return exists
}

func (t *txLog) recordTx(tx *txsystem.Transaction, b *block.Block) error {
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
	proof, err := block.NewPrimaryProof(genericBlock, gtx.UnitID(), crypto.SHA256)
	if err != nil {
		return nil, err
	}
	blockProof, err := NewBlockProof(txPb, proof, b.BlockNumber)
	if err != nil {
		return nil, err
	}
	switch tx := gtx.(type) {
	case money.Transfer:
		return &Bill{
			Id:         tx.UnitID(),
			Value:      tx.TargetValue(),
			TxHash:     tx.Hash(crypto.SHA256),
			BlockProof: blockProof,
		}, nil
	case money.Split:
		return &Bill{
			Id:         utiltx.SameShardID(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256)),
			Value:      tx.Amount(),
			TxHash:     tx.Hash(crypto.SHA256),
			BlockProof: blockProof,
		}, nil
	case money.TransferDC:
		return nil, errors.New("recorded dc tx (should not happen as we only send transfer and split txs)")
	case money.Swap:
		return nil, errors.New("recorded swap tx (should not happen as we only send transfer and split txs)")
	default:
		return nil, errors.New("recorded unknown transaction type (should not happen as we only send transfer and split txs)")
	}
}

func (t *txLog) isAllTxsConfirmed() bool {
	for _, v := range t.txsMap {
		if v == nil {
			return false
		}
	}
	return true
}

func (t *txLog) getAllRecordedBills() []*Bill {
	var bills []*Bill
	for _, v := range t.txsMap {
		if v != nil {
			bills = append(bills, v)
		}
	}
	return bills
}
