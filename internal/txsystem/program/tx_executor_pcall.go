package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/internal/wvm/abruntime"
)

func handlePCallTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[PCallAttributes] {
	return func(tx *types.TransactionOrder, attr *PCallAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing pcall %X", tx.UnitID())
		if err := validatePCallTx(tx, attr, systemIdentifier); err != nil {
			return nil, fmt.Errorf("invalid pcall tx, %w", err)
		}
		unit, err := state.GetUnit(util.BytesToUint256(tx.UnitID()))
		if err != nil {
			return nil, fmt.Errorf("failed to load program '%X': %w", tx.UnitID(), err)
		}
		prog, ok := unit.Data.(*Program)
		if !ok {
			return nil, fmt.Errorf("unit %X does not contain program data type", tx.UnitID())
		}
		if len(prog.Wasm()) < 1 {
			return nil, fmt.Errorf("unit %X does not contain wasm code", tx.UnitID())
		}
		execCtx, err := NewExecCtxFormPCall(tx, attr, prog.progParams, hashAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("program call tx failed, %w", err)
		}
		s, err := NewStateStorage(state, execCtx)
		if err != nil {
			return nil, fmt.Errorf("program call tx failed, %w", err)
		}
		return &types.ServerMetadata{}, abruntime.Call(ctx, prog.Wasm(), attr.FuncName, execCtx, s)
	}
}

func validatePCallTx(tx *types.TransactionOrder, attr *PCallAttributes, sysID []byte) error {
	// TODO verify type from extended identifier (AB-416)
	if !bytes.Equal(sysID, tx.SystemID()) {
		return fmt.Errorf("tx system id does not match tx system id")
	}
	if len(tx.OwnerProof) != 0 {
		return fmt.Errorf("owner proof present")
	}
	if len(attr.FuncName) < 1 {
		return fmt.Errorf("program call is missing function")
	}
	if attr.InputData == nil {
		return fmt.Errorf("program input is missing")
	}
	return nil
}
