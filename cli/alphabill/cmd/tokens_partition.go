package cmd

import (
	"fmt"
	"strconv"

	tokenssdk "github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/predicates/wasm"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	tokenc "github.com/alphabill-org/alphabill/txsystem/tokens/encoder"
)

type (
	TokensPartition struct {
		partitionTypeID types.PartitionTypeID
	}
)

func NewTokensPartition() *TokensPartition {
	return &TokensPartition{
		partitionTypeID: tokenssdk.PartitionTypeID,
	}
}
func (p *TokensPartition) PartitionTypeID() types.PartitionTypeID {
	return p.partitionTypeID
}

func (p *TokensPartition) PartitionTypeIDString() string {
	return "tokens"
}

func (p *TokensPartition) DefaultPartitionParams(flags *ShardConfGenerateFlags) map[string]string {
	partitionParams := make(map[string]string, 1)

	partitionParams[tokensAdminOwnerPredicate] = flags.TokensAdminOwnerPredicate
	partitionParams[tokensFeelessMode] = strconv.FormatBool(flags.TokensFeelessMode)

	return partitionParams
}

func (p *TokensPartition) NewGenesisState(pdr *types.PartitionDescriptionRecord) (*state.State, error) {
	return state.NewEmptyState(), nil
}

func (p *TokensPartition) CreateTxSystem(flags *ShardNodeRunFlags, nodeConf *partition.NodeConf) (txsystem.TransactionSystem, error) {
	stateFilePath := flags.PathWithDefault(flags.StateFile, StateFileName)
	state, header, err := loadStateFile(stateFilePath, func(ui types.UnitID) (types.UnitData, error) {
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
	enc, err := encoder.New(nodeConf.PartitionID(), tokenc.RegisterTxAttributeEncoders, tokenc.RegisterUnitDataEncoders, tokenc.RegisterAuthProof)
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
	wasmEng, err := wasm.New(enc, tpe.Execute, nodeConf.Orchestration(), nodeConf.Observability())
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
		tokens.WithExecutedTransactions(header.ExecutedTransactions),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create tokens tx system: %w", err)
	}
	return txs, nil
}
