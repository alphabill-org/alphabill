package program

import (
	"bytes"
	"context"
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/internal/wvm/abruntime"
)

func handlePDeployTx(ctx context.Context, state *rma.Tree, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[PDeployAttributes] {
	return func(tx *types.TransactionOrder, attr *PDeployAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing pdeploy tx %X", tx.UnitID())
		if err := validateDeployTx(tx, attr, systemIdentifier); err != nil {
			return nil, fmt.Errorf("invalid program deploy tx, %w", err)
		}
		if _, err := state.GetUnit(util.BytesToUint256(tx.UnitID())); err == nil {
			return nil, fmt.Errorf("program unit with id '%X' already exists", tx.UnitID())
		}
		execCtx, err := NewExecCtxFormDeploy(tx, attr, hashAlgorithm)
		if err != nil {
			return nil, fmt.Errorf("deloy program tx failed, %w", err)
		}
		if err = abruntime.CheckProgram(ctx, attr.ProgModule, execCtx); err != nil {
			return nil, fmt.Errorf("program check failed, %w", err)
		}
		if err = state.AtomicUpdate(rma.AddItem(
			util.BytesToUint256(tx.UnitID()),
			script.PredicateAlwaysFalse(),
			&Program{wasm: attr.ProgModule, progParams: attr.ProgParams},
			make([]byte, 32))); err != nil {
			return nil, fmt.Errorf("failed to save program to state, %w", err)
		}
		return &types.ServerMetadata{}, nil
	}
}

func validateDeployTx(tx *types.TransactionOrder, attr *PDeployAttributes, sysID []byte) error {
	// TODO verify type from extended identifier (AB-416)
	if len(tx.OwnerProof) != 0 {
		return fmt.Errorf("owner proof present")
	}
	if !bytes.Equal(sysID, tx.SystemID()) {
		return fmt.Errorf("tx system id does not match tx system id")
	}
	if len(attr.ProgModule) < 1 {
		return fmt.Errorf("wasm module is missing")
	}
	if attr.ProgParams == nil {
		return fmt.Errorf("program init data is missing")
	}
	return nil
}
