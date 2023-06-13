package money

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

// txLog helper struct used to track pending/confirmed transactions
type txLog struct {
	txsMap map[string]*wallet.Proof
}

func NewTxLog(pendingTxs []*types.TransactionOrder) *txLog {
	txsMap := make(map[string]*wallet.Proof, len(pendingTxs))
	for _, tx := range pendingTxs {
		payloadBytes, err := tx.PayloadBytes()
		if err != nil {
			panic(err) // TODO
		}
		txsMap[string(payloadBytes)] = nil
	}
	return &txLog{txsMap: txsMap}
}
func (t *txLog) Contains(tx *types.TransactionRecord) bool {
	payloadBytes, err := tx.TransactionOrder.PayloadBytes()
	if err != nil {
		panic(err) // TODO
	}
	_, exists := t.txsMap[string(payloadBytes)]
	return exists
}

func (t *txLog) RecordTx(tx *types.TransactionRecord, txIdx int, b *types.Block) error {
	proof, _, err := types.NewTxProof(b, txIdx, crypto.SHA256)
	if err != nil {
		return err
	}
	payloadBytes, err := tx.TransactionOrder.PayloadBytes()
	if err != nil {
		panic(err) // TODO
	}
	t.txsMap[string(payloadBytes)] = &wallet.Proof{
		TxRecord: tx,
		TxProof:  proof,
	}
	return nil
}

func (t *txLog) IsAllTxsConfirmed() bool {
	for _, v := range t.txsMap {
		if v == nil {
			return false
		}
	}
	return true
}

func (t *txLog) GetAllRecordedProofs() []*wallet.Proof {
	var proofs []*wallet.Proof
	for _, v := range t.txsMap {
		if v != nil {
			proofs = append(proofs, v)
		}
	}
	return proofs
}
