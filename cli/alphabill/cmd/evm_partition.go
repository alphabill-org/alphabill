package cmd

import (
	"fmt"
	"math/big"
	"strconv"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/api"
)

type (
	EvmPartition struct {
		partitionTypeID types.PartitionTypeID
	}
)

func NewEvmPartition() *EvmPartition {
	return &EvmPartition{
		partitionTypeID: evmsdk.PartitionTypeID,
	}
}
func (p *EvmPartition) PartitionTypeID() types.PartitionTypeID {
	return p.partitionTypeID
}

func (p *EvmPartition) PartitionTypeIDString() string {
	return "evm"
}

func (p *EvmPartition) DefaultPartitionParams(flags *ShardConfGenerateFlags) map[string]string {
	partitionParams := make(map[string]string, 1)

	partitionParams[evmGasUnitPrice] = strconv.FormatUint(evm.DefaultGasPrice, 10)
	partitionParams[evmBlockGasLimit] = strconv.FormatUint(evm.DefaultBlockGasLimit, 10)

	return partitionParams
}

func (p *EvmPartition) NewGenesisState(pdr *types.PartitionDescriptionRecord) (*state.State, error) {
	return state.NewEmptyState(), nil
}

func (p *EvmPartition) CreateTxSystem(flags *ShardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.PathWithDefault(flags.StateFile, StateFileName)
	state, header, err := loadStateFile(stateFilePath, evm.NewUnitData)
	if err != nil {
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}
	params, err := ParseEvmPartitionParams(nodeConf.ShardConf())
	if err != nil {
		return nil, fmt.Errorf("failed to validate evm partition params: %w", err)
	}
	txs, err := evm.NewEVMTxSystem(
		nodeConf.ShardConf().NetworkID,
		nodeConf.ShardConf().PartitionID,
		nodeConf.Observability(),
		evm.WithBlockGasLimit(params.BlockGasLimit),
		evm.WithGasPrice(params.GasUnitPrice),
		evm.WithBlockDB(nodeConf.BlockStore()),
		evm.WithTrustBase(nodeConf.TrustBase()),
		evm.WithState(state),
		evm.WithExecutedTransactions(header.ExecutedTransactions),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create evm tx system: %w", err)
	}

	flags.ServerConfiguration.Router = api.NewAPI(
		state,
		nodeConf.ShardConf().PartitionID,
		big.NewInt(0).SetUint64(params.BlockGasLimit),
		params.GasUnitPrice,
		nodeConf.Observability().Logger(),
	)
	return txs, nil
}
