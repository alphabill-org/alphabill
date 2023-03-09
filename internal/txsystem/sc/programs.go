package sc

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/holiman/uint256"
)

// TODO current state supports 256 bit unit identifiers (AB-416)
var unlockMoneyBillProgramID = append(make([]byte, 24), []byte{0, 0, 0, 0, 0, 's', 'y', 's'}...)

type (
	BuiltInPrograms map[string]BuiltInProgram

	BuiltInProgram func(ctx *ProgramExecutionContext) error

	ProgramExecutionContext struct {
		systemIdentifier []byte
		hashAlgorithm    crypto.Hash
		state            *rma.Tree
		txOrder          *SCallTransactionOrder
	}
)

func initBuiltInPrograms(state *rma.Tree) (BuiltInPrograms, error) {
	if state == nil {
		return nil, errors.New("state is nil")
	}
	if err := state.AtomicUpdate(
		rma.AddItem(
			uint256.NewInt(0).SetBytes(unlockMoneyBillProgramID),
			script.PredicateAlwaysFalse(),
			&Program{},
			make([]byte, 32)),
	); err != nil {
		return nil, fmt.Errorf("failed to add 'unlock money bill' program to the state: %w", err)
	}
	state.Commit()
	return BuiltInPrograms{string(unlockMoneyBillProgramID): unlockMoneyBill}, nil
}

func unlockMoneyBill(*ProgramExecutionContext) error {
	// TODO implement after we have unit proofs (AB-602)
	return errors.New("not implemented")
}
