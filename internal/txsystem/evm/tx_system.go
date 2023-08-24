package evm

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
)

func NewEVMTxSystem(systemIdentifier []byte, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options := DefaultOptions()
	for _, option := range opts {
		option(options)
	}
	if options.state == nil {
		return nil, fmt.Errorf("evm tx sysrem init failed, state tree is nil")
	}
	evm, err := NewEVMModule(systemIdentifier, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM module: %w", err)
	}
	fees, err := newFeeModule(systemIdentifier, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM fee module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		[]txsystem.Module{evm, fees},
		txsystem.WithBeginBlockFunctions(evm.StartBlockFunc(options.blockGasLimit)...),
		txsystem.WithSystemIdentifier(systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
