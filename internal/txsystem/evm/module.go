package evm

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/logger"
	"github.com/ethereum/go-ethereum/core"
)

var _ txsystem.Module = (*Module)(nil)

type (
	Module struct {
		systemIdentifier []byte
		options          *Options
		blockGasCounter  *core.GasPool
		log              *slog.Logger
	}
)

func NewEVMModule(systemIdentifier []byte, opts *Options, log *slog.Logger) (*Module, error) {
	if opts.gasUnitPrice == nil {
		return nil, fmt.Errorf("evm init failed, gas price is nil")
	}
	return &Module{
		systemIdentifier: systemIdentifier,
		options:          opts,
		blockGasCounter:  new(core.GasPool).AddGas(opts.blockGasLimit),
		log:              log,
	}, nil
}

func (m *Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		PayloadTypeEVMCall: handleEVMTx(m.systemIdentifier, m.options, m.blockGasCounter, m.options.blockDB, m.log),
	}
}

func (m *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
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