package sc

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = &SmartContractModule{}

type SmartContractModule struct {
	state            *state.State
	systemIdentifier []byte
	hashAlgorithm    gocrypto.Hash
	programs         BuiltInPrograms
}

func NewSmartContractModule(systemIdentifier []byte, options *Options) (txsystem.Module, error) {
	programs, err := initBuiltInPrograms(options.state)
	if err != nil {
		return nil, fmt.Errorf("failed to init built-in programs: %w", err)
	}
	return &SmartContractModule{
		systemIdentifier: systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            options.state,
		programs:         programs,
	}, nil
}

func (s *SmartContractModule) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		"scall": handleSCallTx(s.state, s.programs, s.systemIdentifier, s.hashAlgorithm),
	}
}

func (s *SmartContractModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		return txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{
			Tx:               ctx.Tx,
			Unit:             nil, // SC transactions do not have owner proofs.
			SystemIdentifier: ctx.SystemIdentifier,
			BlockNumber:      ctx.BlockNumber,
		})
	}

}
