package sc

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

func handleSCallTx(state *rma.Tree, programs BuiltInPrograms, systemIdentifier []byte, hashAlgorithm crypto.Hash) txsystem.GenericExecuteFunc[*SCallTransactionOrder] {
	return func(txOrder *SCallTransactionOrder, currentBlockNr uint64) error {
		// TODO verify type from extended identifier (AB-416)
		if len(txOrder.OwnerProof()) != 0 {
			return fmt.Errorf("owner proof present")
		}

		programUnitID := txOrder.UnitID()
		unit, err := state.GetUnit(programUnitID)
		if err != nil {
			return fmt.Errorf("failed to load program '%X': %w", util.Uint256ToBytes(programUnitID), err)
		}

		if _, ok := unit.Data.(*Program); !ok {
			return fmt.Errorf("unit %X does not contain program data", util.Uint256ToBytes(programUnitID))
		}

		programFunc, f := programs[string(util.Uint256ToBytes(programUnitID))]
		if !f {
			return fmt.Errorf("built-in program '%X' not found", util.Uint256ToBytes(programUnitID))
		}
		ctx := &ProgramExecutionContext{
			systemIdentifier: systemIdentifier,
			hashAlgorithm:    hashAlgorithm,
			state:            state,
			txOrder:          txOrder,
		}

		if err = programFunc(ctx); err != nil {
			return fmt.Errorf("program '%X' execution failed: %w", util.Uint256ToBytes(programUnitID), err)
		}
		return nil
	}
}
