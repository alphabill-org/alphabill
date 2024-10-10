package rpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	StateAPI struct {
		node       partitionNode
		ownerIndex partition.IndexReader
	}

	partitionNode interface {
		NetworkID() types.NetworkID
		SystemID() types.SystemID
		SubmitTx(ctx context.Context, tx *types.TransactionOrder) ([]byte, error)
		GetBlock(ctx context.Context, blockNr uint64) (*types.Block, error)
		LatestBlockNumber() (uint64, error)
		GetTransactionRecordProof(ctx context.Context, hash []byte) (*types.TxRecordProof, error)
		GetLatestRoundNumber(ctx context.Context) (uint64, error)
		TransactionSystemState() txsystem.StateReader
		ValidatorNodes() peer.IDSlice
		GetTrustBase(epochNumber uint64) (types.RootTrustBase, error)
	}

	Unit[T any] struct {
		NetworkID      types.NetworkID       `json:"networkId"`
		SystemID       types.SystemID        `json:"systemId"`
		UnitID         types.UnitID          `json:"unitId"`
		Data           T                     `json:"data"`
		OwnerPredicate types.Bytes           `json:"ownerPredicate,omitempty"`
		StateProof     *types.UnitStateProof `json:"stateProof,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecordProof types.Bytes `json:"txRecordProof"` // hex encoded CBOR of types.TxRecordProof
	}
)

func NewStateAPI(node partitionNode, ownerIndex partition.IndexReader) *StateAPI {
	return &StateAPI{node: node, ownerIndex: ownerIndex}
}

// GetRoundNumber returns the round number of the latest UC seen by node.
func (s *StateAPI) GetRoundNumber(ctx context.Context) (types.Uint64, error) {
	roundNumber, err := s.node.GetLatestRoundNumber(ctx)
	if err != nil {
		return 0, err
	}
	return types.Uint64(roundNumber), nil
}

// GetUnit returns unit data and optionally the state proof for the given unitID.
func (s *StateAPI) GetUnit(unitID types.UnitID, includeStateProof bool) (*Unit[any], error) {
	state := s.node.TransactionSystemState()

	unit, err := state.GetUnit(unitID, true)
	if err != nil {
		if errors.Is(err, avl.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	resp := &Unit[any]{
		NetworkID:      s.node.NetworkID(),
		SystemID:       s.node.SystemID(),
		UnitID:         unitID,
		Data:           unit.Data(),
		OwnerPredicate: unit.Owner(),
		StateProof:     nil,
	}

	if includeStateProof {
		stateProof, err := state.CreateUnitStateProof(unitID, unit.LastLogIndex())
		if err != nil {
			return nil, fmt.Errorf("failed to generate unit state proof: %w", err)
		}
		resp.StateProof = stateProof
	}

	return resp, nil
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (s *StateAPI) GetUnitsByOwnerID(ownerID types.Bytes) ([]types.UnitID, error) {
	if s.ownerIndex == nil {
		return nil, errors.New("owner indexer is disabled")
	}
	unitIds, err := s.ownerIndex.GetOwnerUnits(ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load owner units: %w", err)
	}
	return unitIds, nil
}

// SendTransaction broadcasts the given transaction to the network, returns the submitted transaction hash.
func (s *StateAPI) SendTransaction(ctx context.Context, txBytes types.Bytes) (types.Bytes, error) {
	var tx *types.TransactionOrder
	if err := types.Cbor.Unmarshal(txBytes, &tx); err != nil {
		return nil, fmt.Errorf("failed to decode transaction: %w", err)
	}
	txHash, err := s.node.SubmitTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction to the network: %w", err)
	}
	return txHash, nil
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
func (s *StateAPI) GetTransactionProof(ctx context.Context, txHash types.Bytes) (*TransactionRecordAndProof, error) {
	txRecordProof, err := s.node.GetTransactionRecordProof(ctx, txHash)
	if err != nil {
		if errors.Is(err, partition.ErrIndexNotFound) || errors.Is(err, types.ErrBlockIsNil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load tx record: %w", err)
	}
	txRecordProofCBOR, err := types.Cbor.Marshal(txRecordProof)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx record: %w", err)
	}
	return &TransactionRecordAndProof{
		TxRecordProof: txRecordProofCBOR,
	}, nil
}

// GetBlock returns block for the given block number.
func (s *StateAPI) GetBlock(ctx context.Context, blockNumber types.Uint64) (types.Bytes, error) {
	block, err := s.node.GetBlock(ctx, uint64(blockNumber))
	if err != nil {
		return nil, fmt.Errorf("failed to load block: %w", err)
	}
	if block == nil {
		return nil, nil
	}
	blockCbor, err := types.Cbor.Marshal(block)
	if err != nil {
		return nil, fmt.Errorf("failed to encode block: %w", err)
	}
	return blockCbor, nil
}

// GetTrustBase returns trust base for the given epoch.
func (s *StateAPI) GetTrustBase(epochNumber types.Uint64) (types.RootTrustBase, error) {
	trustBase, err := s.node.GetTrustBase(uint64(epochNumber))
	if err != nil {
		return nil, fmt.Errorf("failed to load trust base: %w", err)
	}
	return trustBase, nil
}
