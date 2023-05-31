package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/internal/wvm/abruntime"
)

func handlePCallTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*PCallTransactionOrder] {
	return func(txOrder *PCallTransactionOrder, currentBlockNr uint64) error {
		logger.Debug("Processing pcall %X", txOrder.UnitID().Bytes())
		if err := validatePCallTx(txOrder, systemIdentifier); err != nil {
			return fmt.Errorf("invalid pcall tx, %w", err)
		}
		unit, err := state.GetUnit(txOrder.UnitID())
		if err != nil {
			return fmt.Errorf("failed to load program '%X': %w", util.Uint256ToBytes(txOrder.UnitID()), err)
		}
		prog, ok := unit.Data.(*Program)
		if !ok {
			return fmt.Errorf("unit %X does not contain program data type", util.Uint256ToBytes(txOrder.UnitID()))
		}
		if len(prog.Wasm()) < 1 {
			return fmt.Errorf("unit %X does not contain wasm code", util.Uint256ToBytes(txOrder.UnitID()))
		}
		execCtx, err := NewExecCtxFormPCall(txOrder, prog.progParams, hashAlgorithm)
		if err != nil {
			return fmt.Errorf("program call tx failed, %w", err)
		}
		s, err := NewStateStorage(state, execCtx)
		if err != nil {
			return fmt.Errorf("program call tx failed, %w", err)
		}
		return abruntime.Call(ctx, prog.Wasm(), txOrder.attributes.Function, execCtx, s)
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
