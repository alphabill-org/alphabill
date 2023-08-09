package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/ethereum/go-ethereum/core"
)

var _ txsystem.Module = (*Module)(nil)

type (
	Module struct {
		systemIdentifier []byte
		options          *Options
		blockGasCounter  *core.GasPool
	}
)

func NewEVMModule(systemIdentifier []byte, opts *Options) (*Module, error) {
	if opts.gasUnitPrice == nil {
		return nil, fmt.Errorf("evm init failed, gas price is nil")
	}
	return &Module{
		systemIdentifier: systemIdentifier,
		options:          opts,
		blockGasCounter:  new(core.GasPool).AddGas(opts.blockGasLimit),
	}, nil
}

func (m *Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		PayloadTypeEVMCall: handleEVMTx(m.systemIdentifier, m.options, m.blockGasCounter),
	}
}

func (m *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
}

func (m *Module) StartBlockFunc(blockGasLimit uint64) []func(blockNr uint64) {
	return []func(blockNr uint64){
		func(blockNr uint64) {
			// reset block gas limit
			log.Trace("previous block gas limit: %v, used %v", m.blockGasCounter.Gas(), blockGasLimit-m.blockGasCounter.Gas())
			*m.blockGasCounter = core.GasPool(blockGasLimit)
		},
	}
}
