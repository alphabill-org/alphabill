package orchestration

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

func NewTxSystem(pdr types.PartitionDescriptionRecord, shardID types.ShardID, observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
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
		pdr,
		shardID,
		options.trustBase,
		[]txtypes.Module{module},
		observe,
		txsystem.WithHashAlgorithm(options.hashAlgorithm),
		txsystem.WithState(options.state),
	)
}

func NOPFeeCreditValidator(_ txtypes.ExecutionContext, _ *types.TransactionOrder) error {
	return nil
}
