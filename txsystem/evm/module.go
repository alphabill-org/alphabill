package evm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/ethereum/go-ethereum/core"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/txsystem"
)

var _ txtypes.Module = (*Module)(nil)

type (
	Module struct {
		systemIdentifier types.SystemID
		options          *Options
		blockGasCounter  *core.GasPool
		execPredicate    predicates.PredicateRunner
		log              *slog.Logger
	}
)

func NewEVMModule(systemIdentifier types.SystemID, opts *Options, log *slog.Logger) (*Module, error) {
	if opts.gasUnitPrice == nil {
		return nil, fmt.Errorf("evm init failed, gas price is nil")
	}
	return &Module{
		systemIdentifier: systemIdentifier,
		options:          opts,
		blockGasCounter:  new(core.GasPool).AddGas(opts.blockGasLimit),
		execPredicate:    predicates.NewPredicateRunner(opts.execPredicate),
		log:              log,
	}, nil
}

func (m *Module) GenericTransactionValidator() genericTransactionValidator {
	return func(ctx *TxValidationContext) error {
		if ctx.Tx.NetworkID != ctx.NetworkID {
			return fmt.Errorf("invalid network id: %d (expected %d)", ctx.Tx.NetworkID, ctx.NetworkID)
		}
		if ctx.Tx.SystemID != ctx.SystemID {
			return txsystem.ErrInvalidSystemIdentifier
		}
		if ctx.BlockNumber >= ctx.Tx.Timeout() {
			return txsystem.ErrTransactionExpired
		}
		return nil
	}
}

func (m *Module) StartBlockFunc(blockGasLimit uint64) []func(blockNr uint64) error {
	return []func(blockNr uint64) error{
		func(blockNr uint64) error {
			// reset block gas limit
			m.log.LogAttrs(context.Background(), logger.LevelTrace, fmt.Sprintf("previous block gas limit: %v, used %v", m.blockGasCounter.Gas(), blockGasLimit-m.blockGasCounter.Gas()), logger.Round(blockNr))
			*m.blockGasCounter = core.GasPool(blockGasLimit)
			return nil
		},
	}
}

func (m *Module) TxHandlers() map[uint16]txtypes.TxExecutor {
	return map[uint16]txtypes.TxExecutor{
		evm.TransactionTypeEVMCall: txtypes.NewTxHandler[evm.TxAttributes, evm.TxAuthProof](m.validateEVMTx, m.executeEVMTx),
	}
}
