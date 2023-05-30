package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/wvm/wasmruntime"
)

func handlePCallTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*PCallTransactionOrder] {
	return func(txOrder *PCallTransactionOrder, currentBlockNr uint64) error {
		logger.Debug("Processing pcall %X", txOrder.UnitID().Bytes())
		if err := validatePCallTx(txOrder, systemIdentifier); err != nil {
			return fmt.Errorf("invalid pcall tx, %w", err)
		}
		execCtx, err := NewExecCtxFormPCall(txOrder, state, hashAlgorithm)
		if err != nil {
			return fmt.Errorf("program call tx failed, %w", err)
		}
		s, err := NewStateStorage(state, execCtx)
		if err != nil {
			return fmt.Errorf("program call tx failed, %w", err)
		}
		return wasmruntime.Call(ctx, execCtx, txOrder.attributes.Function, s)
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
	if tx.attributes.Input == nil {
		return fmt.Errorf("program input is missing")
	}
	return nil
}
