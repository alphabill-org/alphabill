package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
)

type TokenTxConverter struct {
}

func (*TokenTxConverter) ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	feeTx, err := transactions.NewPartitionFeeCreditTx(tx)
	if err != nil {
		return nil, err
	}
	if feeTx != nil {
		return feeTx, nil
	}
	return tokens.NewGenericTx(tx)
}
