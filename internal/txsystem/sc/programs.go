package sc

import (
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/types"
)

// TODO current state supports 256 bit unit identifiers (AB-416)
var unlockMoneyBillProgramID = append(make([]byte, 24), []byte{0, 0, 0, 0, 0, 's', 'y', 's'}...)

type (
	BuiltInPrograms map[string]BuiltInProgram

	BuiltInProgram func(ctx *ProgramExecutionContext) error

	ProgramExecutionContext struct {
		systemIdentifier []byte
		hashAlgorithm    crypto.Hash
		state            *state.State
		txOrder          *types.TransactionOrder
	}
)

func initBuiltInPrograms(s *state.State) (BuiltInPrograms, error) {
	if s == nil {
		return nil, errors.New("state is nil")
	}
	if err := s.Apply(
		state.AddUnit(
			unlockMoneyBillProgramID,
			script.PredicateAlwaysFalse(),
			&Program{},
		),
	); err != nil {
		return nil, fmt.Errorf("failed to add 'unlock money bill' program to the state: %w", err)
	}
	if _, _, err := s.CalculateRoot(); err != nil {
		return nil, fmt.Errorf("unable to calculat root hash: %w", err)
	}
	if err := s.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit 'unlock money bill' program to the state: %w", err)
	}
	return BuiltInPrograms{string(unlockMoneyBillProgramID): unlockMoneyBill}, nil
}

func unlockMoneyBill(*ProgramExecutionContext) error {
	// TODO implement after we have unit proofs (AB-602)
	return errors.New("not implemented")
}
