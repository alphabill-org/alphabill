package money

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
)

var txConverter = &TxConverter{}

type TxConverter struct {
}

func (t *TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	if tx == nil {
		return nil, errors.New("cannot convert tx: tx is nil")
	}
	return money.NewMoneyTx(alphabillMoneySystemId, tx)
}
