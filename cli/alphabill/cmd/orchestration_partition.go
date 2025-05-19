package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	orchestrationsdk "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"
)

type (
	OrchestrationPartition struct {
		partitionTypeID types.PartitionTypeID
	}
)

func NewOrchestrationPartition() *OrchestrationPartition {
	return &OrchestrationPartition{
		partitionTypeID: orchestrationsdk.PartitionTypeID,
	}
}
func (p *OrchestrationPartition) PartitionTypeID() types.PartitionTypeID {
	return p.partitionTypeID
}

func (p *OrchestrationPartition) PartitionTypeIDString() string {
	return "orchestration"
}

func (p *OrchestrationPartition) DefaultPartitionParams(flags *ShardConfGenerateFlags) map[string]string {
	partitionParams := make(map[string]string, 1)
	alwaysTruePredicate := string(hex.Encode(templates.AlwaysTrueBytes()))

	op := flags.OrchestrationOwnerPredicate
	if op == "" {
		op = alwaysTruePredicate
	}
	partitionParams[orchestrationOwnerPredicate] = op

	return partitionParams
}

func (p *OrchestrationPartition) NewGenesisState(pdr *types.PartitionDescriptionRecord) (*state.State, error) {
	return state.NewEmptyState(), nil
}

func (p *OrchestrationPartition) CreateTxSystem(flags *ShardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.PathWithDefault(flags.StateFile, StateFileName)
	state, header, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return moneysdk.NewUnitData(ui, nodeConf.ShardConf())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}

	params, err := ParseOrchestrationPartitionParams(nodeConf.ShardConf())
	if err != nil {
		return nil, fmt.Errorf("failed to validate orchestration partition params: %w", err)
	}

	txs, err := orchestration.NewTxSystem(
		*nodeConf.ShardConf(),
		nodeConf.Observability(),
		orchestration.WithHashAlgorithm(nodeConf.HashAlgorithm()),
		orchestration.WithState(state),
		orchestration.WithOwnerPredicate(params.OwnerPredicate),
		orchestration.WithExecutedTransactions(header.ExecutedTransactions),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create money tx system: %w", err)
	}
	return txs, err
}

func loadStateFile(stateFilePath string, unitDataConstructor state.UnitDataConstructor) (*state.State, *state.Header, error) {
	if !util.FileExists(stateFilePath) {
		return nil, nil, fmt.Errorf("state file '%s' not found", stateFilePath)
	}

	stateFile, err := os.Open(filepath.Clean(stateFilePath))
	if err != nil {
		return nil, nil, err
	}
	defer stateFile.Close()

	s, header, err := state.NewRecoveredState(stateFile, unitDataConstructor)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build state tree from state file: %w", err)
	}
	return s, header, nil
}
