package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

func handlePDeployTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*PDeployTransactionOrder] {
	return func(txOrder *PDeployTransactionOrder, currentBlockNr uint64) error {
		if err := validateDeployTx(txOrder, systemIdentifier); err != nil {
			return fmt.Errorf("invalid program deploy tx, %w", err)
		}
		programUnitID := txOrder.UnitID()
		// calculate hash of transaction
		h := txOrder.Hash(hashAlgorithm)
		return ExecuteAndDeploy(ctx, programUnitID, txOrder.attributes.Program, txOrder.attributes.InitData, state, h)
	}
}

func validateDeployTx(tx *PDeployTransactionOrder, sysID []byte) error {
	// TODO verify type from extended identifier (AB-416)
	// TODO: owner proof is missing for programs
	if !bytes.Equal(sysID, tx.SystemID()) {
		return fmt.Errorf("tx system id does not match tx system id")
	}
	if len(tx.attributes.Program) < 1 {
		return fmt.Errorf("wasm module is missing")
	}
	if len(tx.attributes.InitData) < 1 {
		return fmt.Errorf("program input is missing")
	}
	return nil
}
