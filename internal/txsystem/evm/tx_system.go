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
	evm, err := NewEVMModule(systemIdentifier, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load EVM module: %w", err)
	}
	// TODO add fee credit support
	return txsystem.NewGenericTxSystem(
		[]txsystem.Module{evm},
		txsystem.WithSystemIdentifier(systemIdentifier),
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}
