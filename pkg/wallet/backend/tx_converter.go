package backend

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
)

func NewTxConverter(systemId []byte) *TxConverter {
	return &TxConverter{systemId: systemId}
}

type TxConverter struct {
	systemId []byte
}

func (t *TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	if tx == nil {
		return nil, errors.New("cannot convert tx: tx is nil")
	}
	return money.NewMoneyTx(t.systemId, tx)
}
