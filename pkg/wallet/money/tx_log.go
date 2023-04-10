package money

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	utiltx "github.com/alphabill-org/alphabill/internal/txsystem/util"
)

// txLog helper struct used to track pending/confirmed transactions
type txLog struct {
	txsMap map[string]*BlockProof
}

func NewTxLog(pendingTxs []*txsystem.Transaction) *txLog {
	txsMap := make(map[string]*BlockProof, len(pendingTxs))
	for _, tx := range pendingTxs {
		txsMap[string(tx.TxBytes())] = nil
	}
	return &txLog{txsMap: txsMap}
}
func (t *txLog) Contains(tx *txsystem.Transaction) bool {
	_, exists := t.txsMap[string(tx.TxBytes())]
	return exists
}

func (t *txLog) RecordTx(gtx txsystem.GenericTransaction, b *block.GenericBlock) error {
	proof, err := t.extractProof(gtx, b)
	if err != nil {
		return err
	}
	t.txsMap[string(gtx.ToProtoBuf().TxBytes())] = proof
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

func (t *txLog) GetAllRecordedProofs() []*BlockProof {
	var proofs []*BlockProof
	for _, v := range t.txsMap {
		if v != nil {
			proofs = append(proofs, v)
		}
	}
	return proofs
}

// extractProof Extracts proof from given transaction and block.
func (t *txLog) extractProof(gtx txsystem.GenericTransaction, b *block.GenericBlock) (*BlockProof, error) {
	var unitID []byte
	switch tx := gtx.(type) {
	case money.Split:
		unitID = utiltx.SameShardIDBytes(tx.UnitID(), tx.HashForIdCalculation(crypto.SHA256))
	default:
		unitID = gtx.ToProtoBuf().UnitId
	}
	proof, err := block.NewPrimaryProof(b, unitID, crypto.SHA256)
	if err != nil {
		return nil, err
	}
	blockProof, err := NewBlockProof(gtx.ToProtoBuf(), proof, b.UnicityCertificate.InputRecord.RoundNumber)
	if err != nil {
		return nil, err
	}
	return blockProof, nil
}
