package rpc

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
)

type StateAPI struct {
	node *partition.Node
}

func NewStateAPI(node *partition.Node) *StateAPI {
	return &StateAPI{node}
}

// Returns the round number of the latest UC seen by node.
func (s *StateAPI) GetRoundNumber(ctx context.Context) (uint64, error) {
	return s.node.GetLatestRoundNumber(ctx)
}

func (s *StateAPI) GetUnit(unitID string) (state.UnitData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StateAPI) GetUnitsByOwnerID(ownerID string) ([]state.UnitData, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *StateAPI) SendTransaction(tx []byte) error {
	return fmt.Errorf("not implemented")
}

func (s *StateAPI) GetTransactionProof(txHash string) ([]byte, error) {
	return nil, fmt.Errorf("not implemented")
}
