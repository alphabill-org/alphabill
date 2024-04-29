package orchestration

import (
	"fmt"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func NewTxSystem(observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions()
	if err != nil {
		return nil, fmt.Errorf("failed to load default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}
	module, err := NewModule(options)
	if err != nil {
		return nil, fmt.Errorf("failed to load module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		options.systemIdentifier,
		NOPFeeCreditValidator,
		options.trustBase,
		[]txsystem.Module{module},
		observe,
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}

func NOPFeeCreditValidator(_ *txsystem.TxExecutionContext, _ *types.TransactionOrder) error {
	return nil
}
