package orchestration

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/txsystem"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
)

const PartitionType = "orchestration"

func NewTxSystem(shardConf types.PartitionDescriptionRecord, observe txsystem.Observability, opts ...Option) (*txsystem.GenericTxSystem, error) {
	options, err := defaultOptions(observe)
	if err != nil {
		return nil, fmt.Errorf("failed to load default configuration: %w", err)
	}
	for _, option := range opts {
		option(options)
	}
	module, err := NewModule(shardConf, options)
	if err != nil {
		return nil, fmt.Errorf("failed to load module: %w", err)
	}
	return txsystem.NewGenericTxSystem(
		shardConf,
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
