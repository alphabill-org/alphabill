package sc

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
)

func handleSCallTx(state *state.State, programs BuiltInPrograms, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[SCallAttributes] {
	return func(tx *types.TransactionOrder, attr *SCallAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		// TODO verify type from extended identifier (AB-416)
		if len(tx.OwnerProof) != 0 {
			return nil, fmt.Errorf("owner proof present")
		}

		programUnitID := tx.UnitID()
		unit, err := state.GetUnit(programUnitID, false)
		if err != nil {
			return nil, fmt.Errorf("failed to load program '%X': %w", programUnitID, err)
		}

		if _, ok := unit.Data().(*Program); !ok {
			return nil, fmt.Errorf("unit %X does not contain program data", programUnitID)
		}

		programFunc, f := programs[string(programUnitID)]
		if !f {
			return nil, fmt.Errorf("built-in program '%X' not found", programUnitID)
		}
		ctx := &ProgramExecutionContext{
			systemIdentifier: systemIdentifier,
			hashAlgorithm:    hashAlgorithm,
			state:            state,
			txOrder:          tx,
		}

		if err = programFunc(ctx); err != nil {
			return nil, fmt.Errorf("program '%X' execution failed: %w", programUnitID, err)
		}
		return &types.ServerMetadata{}, nil
	}
}
