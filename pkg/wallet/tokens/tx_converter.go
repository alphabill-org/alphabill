package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

type TokenTxConverter struct {
}

func (*TokenTxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return tokens.NewGenericTx(tx)
}
