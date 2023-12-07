package tokens

import (
	"bytes"

	"github.com/alphabill-org/alphabill/txsystem"
)

func ValidateGenericTransaction(ctx *txsystem.TxValidationContext) error {
	if !bytes.Equal(ctx.Tx.SystemID(), ctx.SystemIdentifier) {
		return txsystem.ErrInvalidSystemIdentifier
	}
	if ctx.BlockNumber >= ctx.Tx.Timeout() {
		return txsystem.ErrTransactionExpired
	}
	return nil
}
