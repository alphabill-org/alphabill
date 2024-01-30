package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

type (
	StateAPI struct {
		node partitionNode
	}

	UnitData struct {
		DataAndProofCBOR cbor.RawMessage `json:"dataAndProofCbor"`
	}

	Transaction struct {
		TxOrderCbor cbor.RawMessage `json:"txOrderCbor"`
	}

	TransactionRecordAndProof struct {
		TxRecordCbor cbor.RawMessage `json:"txRecordCbor"`
		TxProofCbor  cbor.RawMessage `json:"txProofCbor"`
	}
)

func NewStateAPI(node partitionNode) *StateAPI {
	return &StateAPI{node}
}

// GetRoundNumber returns the round number of the latest UC seen by node.
func (s *StateAPI) GetRoundNumber(ctx context.Context) (uint64, error) {
	return s.node.GetLatestRoundNumber(ctx)
}

// GetUnit returns the unit data for given unitID.
func (s *StateAPI) GetUnit(unitID []byte, returnProof bool, returnData bool) (*UnitData, error) {
	unitState, err := s.node.GetUnitState(unitID, returnProof, returnData)
	if err != nil {
		return nil, fmt.Errorf("failed to load unit: %w", err)
	}
	unitStateCBOR, err := cbor.Marshal(unitState)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal unit state to cbor: %w", err)
	}
	return &UnitData{DataAndProofCBOR: unitStateCBOR}, nil
}

// GetUnitsByOwnerID returns list of unit identifiers that belong to the given owner.
func (s *StateAPI) GetUnitsByOwnerID(ownerID []byte) ([]types.UnitID, error) {
	unitIds, err := s.node.GetOwnerUnits(ownerID)
	if err != nil {
		return nil, fmt.Errorf("failed to load owner units: %w", err)
	}
	return unitIds, nil
}

// SendTransaction broadcasts the given transaction to the network, returns the submitted transaction hash.
func (s *StateAPI) SendTransaction(ctx context.Context, tx *Transaction) ([]byte, error) {
	var txOrder *types.TransactionOrder
	if err := cbor.Unmarshal(tx.TxOrderCbor, &txOrder); err != nil {
		return nil, fmt.Errorf("failed to decode tx order: %w", err)
	}
	txHash, err := s.node.SubmitTx(ctx, txOrder)
	if err != nil {
		return nil, fmt.Errorf("failed to submit transaction to the network: %w", err)
	}
	return txHash, nil
}

// GetTransactionProof returns transaction record and proof for the given transaction hash.
func (s *StateAPI) GetTransactionProof(ctx context.Context, txHash []byte) (*TransactionRecordAndProof, error) {
	txRecord, txProof, err := s.node.GetTransactionRecord(ctx, txHash)
	if err != nil {
		return nil, fmt.Errorf("failed to load tx record: %w", err)
	}
	txRecordCbor, err := cbor.Marshal(txRecord)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx record: %w", err)
	}
	txProofCbor, err := cbor.Marshal(txProof)
	if err != nil {
		return nil, fmt.Errorf("failed to encode tx proof: %w", err)
	}
	return &TransactionRecordAndProof{
		TxRecordCbor: txRecordCbor,
		TxProofCbor:  txProofCbor,
	}, nil
}
