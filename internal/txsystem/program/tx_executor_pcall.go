package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

func handlePCallTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*PCallTransactionOrder] {
	return func(txOrder *PCallTransactionOrder, currentBlockNr uint64) error {
		if err := validatePCallTx(txOrder, systemIdentifier); err != nil {
			return fmt.Errorf("invalid pcall tx, %w", err)
		}
		programUnitID := txOrder.UnitID()
		// calculate hash of transaction
		h := txOrder.Hash(hashAlgorithm)
		return Call(ctx, programUnitID, txOrder.attributes.Function, txOrder.attributes.Input, state, h)
	}
}

func validatePCallTx(tx *PCallTransactionOrder, sysID []byte) error {
	// TODO verify type from extended identifier (AB-416)
	if !bytes.Equal(sysID, tx.SystemID()) {
		return fmt.Errorf("tx system id does not match tx system id")
	}
	if len(tx.OwnerProof()) != 0 {
		return fmt.Errorf("owner proof present")
	}
	if len(tx.attributes.Function) < 1 {
		return fmt.Errorf("program call is missing function")
	}
	if len(tx.attributes.Input) < 1 {
		return fmt.Errorf("program input is missing")
	}
	return nil
}
