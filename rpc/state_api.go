package rpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/types"
)

type (
	StateAPI struct {
		node       partitionNode
		ownerIndex partition.IndexReader
	}

	Unit[T any] struct {
		UnitID         types.UnitID          `json:"unitId"`
		Data           T                     `json:"data"`
		OwnerPredicate types.Bytes           `json:"ownerPredicate,omitempty"`
		StateProof     *types.UnitStateProof `json:"stateProof,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecord types.Bytes `json:"txRecord"`
		TxProof  types.Bytes `json:"txProof"`
	}
)

func NewStateAPI(node partitionNode, ownerIndex partition.IndexReader) *StateAPI {
	return &StateAPI{node: node, ownerIndex: ownerIndex}
}

// GetRoundNumber returns the round number of the latest UC seen by node.
func (s *StateAPI) GetRoundNumber(ctx context.Context) (uint64, error) {
	return s.node.GetLatestRoundNumber(ctx)
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
		UnitID:         unitID,
		Data:           unit.Data(),
		OwnerPredicate: unit.Bearer(),
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
	if err := cbor.Unmarshal(txBytes, &tx); err != nil {
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
	txRecord, txProof, err := s.node.GetTransactionRecord(ctx, txHash)
	if err != nil {
		if errors.Is(err, partition.ErrIndexNotFound) || errors.Is(err, types.ErrBlockIsNil) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to load tx record: %w", err)
	}
	txRecordBytes, err := encodeCbor(txRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx record: %w", err)
	}
	txProofBytes, err := encodeCbor(txProof)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx proof: %w", err)
	}
	return &TransactionRecordAndProof{
		TxRecord: txRecordBytes,
		TxProof:  txProofBytes,
	}, nil
}

// GetBlock returns block for the given block number.
func (s *StateAPI) GetBlock(ctx context.Context, blockNumber uint64) (types.Bytes, error) {
	block, err := s.node.GetBlock(ctx, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to load block: %w", err)
	}
	if block == nil {
		return nil, nil
	}
	blockCbor, err := encodeCbor(block)
	if err != nil {
		return nil, fmt.Errorf("failed to encode block: %w", err)
	}
	return blockCbor, nil
}

func encodeCbor(v interface{}) ([]byte, error) {
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return nil, err
	}
	data, err := enc.Marshal(v)
	if err != nil {
		return nil, err
	}
	return data, nil
}
