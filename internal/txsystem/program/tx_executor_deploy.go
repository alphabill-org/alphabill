package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/vm/wasmruntime"
)

func handlePDeployTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*PDeployTransactionOrder] {
	return func(txOrder *PDeployTransactionOrder, currentBlockNr uint64) error {
		logger.Debug("Processing pdeploy tx %X", txOrder.UnitID().Bytes())

		if err := validateDeployTx(txOrder, systemIdentifier); err != nil {
			return fmt.Errorf("invalid program deploy tx, %w", err)
		}
		execCtx, err := NewExecCtxFormDeploy(txOrder, state, hashAlgorithm)
		if err != nil {
			return fmt.Errorf("deloy program tx failed, %w", err)
		}
		if err = wasmruntime.CheckProgram(ctx, execCtx); err != nil {
			return fmt.Errorf("program check failed, %w", err)
		}
		if err = state.AtomicUpdate(rma.AddItem(
			txOrder.UnitID(),
			script.PredicateAlwaysFalse(),
			&Program{wasm: txOrder.attributes.Program, progParams: txOrder.attributes.InitData},
			make([]byte, 32))); err != nil {
			return fmt.Errorf("failed to save program to state, %w", err)
		}
		return nil
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
	if tx.attributes.InitData == nil {
		return fmt.Errorf("program init data is missing")
	}
	return nil
}
