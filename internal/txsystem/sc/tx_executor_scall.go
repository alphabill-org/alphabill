package sc

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleSCallTx(state *rma.Tree, programs BuiltInPrograms, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[SCallAttributes] {
	return func(tx *types.TransactionOrder, attr *SCallAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		// TODO verify type from extended identifier (AB-416)
		if len(tx.OwnerProof) != 0 {
			return nil, fmt.Errorf("owner proof present")
		}

		programUnitID := util.BytesToUint256(tx.UnitID())
		unit, err := state.GetUnit(programUnitID)
		if err != nil {
			return nil, fmt.Errorf("failed to load program '%X': %w", util.Uint256ToBytes(programUnitID), err)
		}

		if _, ok := unit.Data.(*Program); !ok {
			return nil, fmt.Errorf("unit %X does not contain program data", util.Uint256ToBytes(programUnitID))
		}

		programFunc, f := programs[string(util.Uint256ToBytes(programUnitID))]
		if !f {
			return nil, fmt.Errorf("built-in program '%X' not found", util.Uint256ToBytes(programUnitID))
		}
		ctx := &ProgramExecutionContext{
			systemIdentifier: systemIdentifier,
			hashAlgorithm:    hashAlgorithm,
			state:            state,
			txOrder:          tx,
		}

		if err = programFunc(ctx); err != nil {
			return nil, fmt.Errorf("program '%X' execution failed: %w", util.Uint256ToBytes(programUnitID), err)
		}
		return &types.ServerMetadata{}, nil
	}
}
