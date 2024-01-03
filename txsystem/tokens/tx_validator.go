package tokens

import (
	"github.com/alphabill-org/alphabill/txsystem"
)

func ValidateGenericTransaction(ctx *txsystem.TxValidationContext) error {
	if ctx.Tx.SystemID() != ctx.SystemIdentifier {
		return txsystem.ErrInvalidSystemIdentifier
	}
	if ctx.BlockNumber >= ctx.Tx.Timeout() {
		return txsystem.ErrTransactionExpired
	}
	return nil
}
