package cmd

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	evmsdk "github.com/alphabill-org/alphabill-go-base/txsystem/evm"
	moneysdk "github.com/alphabill-org/alphabill-go-base/txsystem/money"
	orchestrationsdk "github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	tokenssdk "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/evm"
	"github.com/alphabill-org/alphabill/txsystem/evm/api"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/orchestration"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	tokenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

func createTxSystem(flags *shardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	switch nodeConf.ShardConf().PartitionTypeID {
	case moneysdk.PartitionTypeID:
		return newMoneyTxSystem(flags, nodeConf)
	case tokenssdk.PartitionTypeID:
		return newMoneyTxSystem(flags, nodeConf)
	case evmsdk.PartitionTypeID:
		return newMoneyTxSystem(flags, nodeConf)
	case orchestrationsdk.PartitionTypeID:
		return newMoneyTxSystem(flags, nodeConf)
	default:
		return nil, fmt.Errorf("unsupported partition type %d", nodeConf.ShardConf().PartitionTypeID)
	}
}

func newMoneyTxSystem(flags *shardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.pathWithDefault(flags.StateFile, stateFileName)
	state, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return moneysdk.NewUnitData(ui, nodeConf.ShardConf())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}

	txs, err := money.NewTxSystem(
		nodeConf.ShardConf(),
		nodeConf.Observability(),
		money.WithHashAlgorithm(nodeConf.HashAlgorithm()),
		money.WithTrustBase(nodeConf.TrustBase()),
		money.WithState(state),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create money tx system: %w", err)
	}
	return txs, err
}

func newTokensTxSystem(flags *shardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.pathWithDefault(flags.StateFile, stateFileName)
	state, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
		return tokenssdk.NewUnitData(ui, nodeConf.ShardConf())
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load state file: %w", err)
	}

	params, err := ParseTokensPartitionParams(nodeConf.ShardConf())
	if err != nil {
		return nil, fmt.Errorf("failed to validate tokens partition params: %w", err)
	}

	// register all unit- and attribute types from token tx system
	enc, err := encoder.New(tokenc.RegisterTxAttributeEncoders, tokenc.RegisterUnitDataEncoders)
	if err != nil {
		return nil, fmt.Errorf("creating encoders for WASM predicate engine: %w", err)
	}

	// create predicate engine for the transaction system.
	// however, the WASM engine needs to be able to execute predicate templates
	// so we create template engine first and use it for both WASM engine and
	// tx system engine.
	// it's safe to share template engine as it doesn't have internal state and
	// predicate executions happen in serialized manner anyway
	templateEng, err := templates.New(nodeConf.Observability())
	if err != nil {
		return nil, fmt.Errorf("creating predicate templates executor: %w", err)
	}
	tpe, err := predicates.Dispatcher(templateEng)
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor for WASM engine: %w", err)
	}
	wasmEng, err := wasm.New(enc, tpe.Execute, nodeConf.Observability())
	if err != nil {
		return nil, fmt.Errorf("creating predicate WASM executor: %w", err)
	}
	predEng, err := predicates.Dispatcher(templateEng, wasmEng)
	if err != nil {
		return nil, fmt.Errorf("creating predicate executor: %w", err)
	}

	txs, err := tokens.NewTxSystem(
		*nodeConf.ShardConf(),
		nodeConf.Observability(),
		tokens.WithHashAlgorithm(nodeConf.HashAlgorithm()),
		tokens.WithTrustBase(nodeConf.TrustBase()),
		tokens.WithState(state),
		tokens.WithPredicateExecutor(predEng.Execute),
		tokens.WithAdminOwnerPredicate(params.AdminOwnerPredicate),
		tokens.WithFeelessMode(params.FeelessMode),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokens tx system: %w", err)
	}
	return txs, nil
}

func newEvmTxSystem(flags *shardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.pathWithDefault(flags.StateFile, stateFileName)
	state, err := loadStateFile(stateFilePath, evm.NewUnitData)
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

func newOrchestrationTxSystem(flags *shardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.pathWithDefault(flags.StateFile, stateFileName)
	state, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
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
		orchestration.WithTrustBase(nodeConf.TrustBase()),
		orchestration.WithState(state),
		orchestration.WithOwnerPredicate(params.OwnerPredicate),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create money tx system: %w", err)
	}
	return txs, err
}

func loadStateFile(stateFilePath string, unitDataConstructor state.UnitDataConstructor) (*state.State, error) {
	if !util.FileExists(stateFilePath) {
		return nil, fmt.Errorf("state file '%s' not found", stateFilePath)
	}

	stateFile, err := os.Open(filepath.Clean(stateFilePath))
	if err != nil {
		return nil, err
	}
	defer stateFile.Close()

	s, err := state.NewRecoveredState(stateFile, unitDataConstructor)
	if err != nil {
		return nil, fmt.Errorf("failed to build state tree from state file: %w", err)
	}
	return s, nil
}
