package wallet

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
)

type TxConverter struct {
	systemID []byte
}

func NewTxConverter(systemID []byte) *TxConverter {
	return &TxConverter{systemID: systemID}
}

func (t *TxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	if tx == nil {
		return nil, errors.New("cannot convert tx: tx is nil")
	}
	return vd.NewVDTx(t.systemID, tx)
}
