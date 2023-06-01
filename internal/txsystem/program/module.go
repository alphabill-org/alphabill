package program

import (
	"context"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = &Module{}

type Module struct {
	ctx              context.Context
	state            *rma.Tree
	systemIdentifier []byte
	hashAlgorithm    gocrypto.Hash
}

func NewProgramModule(ctx context.Context, systemIdentifier []byte, options *Options) (txsystem.Module, error) {
	if options.state == nil {
		return nil, fmt.Errorf("state is nil")
	}
	return &Module{
		ctx:              ctx,
		systemIdentifier: systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            options.state,
	}, nil
}

func (s *Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		"pdeploy": handlePDeployTx(s.ctx, s.state, s.systemIdentifier, s.hashAlgorithm),
		"pcall":   handlePCallTx(s.ctx, s.state, s.systemIdentifier, s.hashAlgorithm),
	}
}

func (s *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		return txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{
			Tx:               ctx.Tx,
			Unit:             nil, // program transactions do not have owner proofs.
			SystemIdentifier: ctx.SystemIdentifier,
			BlockNumber:      ctx.BlockNumber,
		})
	}

}
