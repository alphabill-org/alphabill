package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

type (
	StateAPI struct {
		node partitionNode
	}

	Unit struct {
		UnitID         types.UnitID          `json:"unitId"`
		Data           state.UnitData        `json:"data"`
		OwnerPredicate types.PredicateBytes  `json:"ownerPredicate,omitempty"`
		StateProof     *types.UnitStateProof `json:"stateProof,omitempty"`
	}

	TransactionRecordAndProof struct {
		TxRecord types.Bytes `json:"txRecord"`
		TxProof  types.Bytes `json:"txProof"`
	}
)

func NewStateAPI(node partitionNode) *StateAPI {
	return &StateAPI{node}
}

// GetRoundNumber returns the round number of the latest UC seen by node.
func (s *StateAPI) GetRoundNumber(ctx context.Context) (uint64, error) {
	return s.node.GetLatestRoundNumber(ctx)
}

// GetUnit returns unit data and optionally the state proof for the given unitID.
func (s *StateAPI) GetUnit(unitID types.UnitID, includeStateProof bool) (*Unit, error) {
	state := s.node.TransactionSystem().State()

	unit, err := state.GetUnit(unitID, true)
	if err != nil {
		return nil, err
	}

	resp := &Unit{
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
	unitIds, err := s.node.GetOwnerUnits(ownerID)
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
