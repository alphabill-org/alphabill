package partitions

import (
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
)

type (
	ValidatorAssignmentRecord struct {
		_           struct{}          `cbor:",toarray"`
		NetworkID   types.NetworkID   `json:"networkId"`
		PartitionID types.PartitionID `json:"partitionId"`
		ShardID     types.ShardID     `json:"shardId"`
		EpochNumber uint64            `json:"epochNo"`
		RoundNumber uint64            `json:"roundNo"`
		Nodes       []*types.NodeInfo `json:"nodes"`
	}
)

// TODO partition genesis record should support multi-shard nodes i.e. currently we create one VAR per partition, but should create one VAR per shard
func NewVARFromGenesis(gpr *genesis.GenesisPartitionRecord) *ValidatorAssignmentRecord {
	pdr := gpr.PartitionDescription
	uc := gpr.Certificate
	return &ValidatorAssignmentRecord{
		NetworkID:   pdr.NetworkID,
		PartitionID: pdr.PartitionID,
		ShardID:     uc.ShardTreeCertificate.Shard,
		EpochNumber: uc.InputRecord.Epoch,
		RoundNumber: uc.InputRecord.RoundNumber,
		Nodes:       NewVarNodesFromGenesisNodes(gpr.Validators),
	}
}

func NewVARNodeFromGenesisNode(pn *genesis.PartitionNode) *types.NodeInfo {
	return &types.NodeInfo{
		NodeID: pn.NodeID,
		SigKey: pn.SignKey,
		Stake:  1,
	}
}

func NewVarNodesFromGenesisNodes(genesisNodes []*genesis.PartitionNode) []*types.NodeInfo {
	nodes := make([]*types.NodeInfo, 0, len(genesisNodes))
	for _, pn := range genesisNodes {
		nodes = append(nodes, NewVARNodeFromGenesisNode(pn))
	}
	return nodes
}

// Verify verifies validator info and that the VAR extends the previous VAR if provided.
func (v *ValidatorAssignmentRecord) Verify(prev *ValidatorAssignmentRecord) error {
	if prev != nil {
		if v.NetworkID != prev.NetworkID {
			return fmt.Errorf("invalid network id, provided %d previous %d", v.NetworkID, prev.NetworkID)
		}
		if v.PartitionID != prev.PartitionID {
			return fmt.Errorf("invalid partition id, provided %d previous %d", v.PartitionID, prev.PartitionID)
		}
		if !v.ShardID.Equal(prev.ShardID) {
			return fmt.Errorf("invalid shard id, provided \"0x%x\" previous \"0x%x\"", v.ShardID.Bytes(), prev.ShardID.Bytes())
		}
		if v.EpochNumber != prev.EpochNumber+1 {
			return fmt.Errorf("invalid epoch number, provided %d previous %d", v.EpochNumber, prev.EpochNumber)
		}
		if v.RoundNumber <= prev.RoundNumber {
			return fmt.Errorf("invalid shard round number, provided %d previous %d", v.RoundNumber, prev.RoundNumber)
		}
	}

	for i, n := range v.Nodes {
		if err := n.IsValid(); err != nil {
			return fmt.Errorf("invalid node at idx %d: %w", i, err)
		}
	}
	return nil
}
