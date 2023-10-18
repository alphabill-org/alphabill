package evm

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

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

	verifyTransactionProof struct {
	}
)

func (v verifyTransactionProof) RequiredGas(input []byte) uint64 {
	return 0
}

func (v verifyTransactionProof) Run(input []byte) ([]byte, error) {
	panic("implement me")
}

func NewEVMModule(systemIdentifier []byte, opts *Options) (*Module, error) {
	if opts.gasUnitPrice == nil {
		return nil, fmt.Errorf("evm init failed, gas price is nil")
	}

	vm.PrecompiledContractsHomestead[common.BytesToAddress([]byte{100})] = &verifyTransactionProof{}
	vm.PrecompiledAddressesHomestead = append(vm.PrecompiledAddressesHomestead, common.BytesToAddress([]byte{100}))
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

func (m *Module) StartBlockFunc(blockGasLimit uint64) []func(blockNr uint64) error {
	return []func(blockNr uint64) error{
		func(blockNr uint64) error {
			// reset block gas limit
			log.Trace("previous block gas limit: %v, used %v", m.blockGasCounter.Gas(), blockGasLimit-m.blockGasCounter.Gas())
			*m.blockGasCounter = core.GasPool(blockGasLimit)
			return nil
		},
	}
}
