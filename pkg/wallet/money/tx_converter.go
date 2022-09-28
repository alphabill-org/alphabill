package money

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
)

var txConverter = &TxConverter{}

type TxConverter struct {
}

func (t *TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return money.NewMoneyTx(alphabillMoneySystemId, tx)
}
