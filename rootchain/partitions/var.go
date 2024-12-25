package partitions

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	ValidatorAssignmentRecord struct {
		_           struct{}          `cbor:",toarray"`
		NetworkID   types.NetworkID   `json:"networkId"`
		PartitionID types.PartitionID `json:"partitionId"`
		ShardID     types.ShardID     `json:"shardId"`
		EpochNumber uint64            `json:"epochNo"`
		RoundNumber uint64            `json:"roundNo"`
		Nodes       []NodeInfo        `json:"nodes"`
	}

	NodeInfo struct {
		_       struct{}  `cbor:",toarray"`
		NodeID  string    `json:"nodeId"`  // libp2p node identifier (derived from auth key)
		SignKey hex.Bytes `json:"signKey"` // alphabill signing key
		AuthKey hex.Bytes `json:"authKey"` // libp2p authentication key
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

func NewVARNodeFromGenesisNode(pn *genesis.PartitionNode) NodeInfo {
	return NodeInfo{
		NodeID:  pn.NodeID,
		AuthKey: pn.AuthKey,
		SignKey: pn.SignKey,
	}
}

func NewVarNodesFromGenesisNodes(genesisNodes []*genesis.PartitionNode) []NodeInfo {
	nodes := make([]NodeInfo, 0, len(genesisNodes))
	for _, pn := range genesisNodes {
		nodes = append(nodes, NewVARNodeFromGenesisNode(pn))
	}
	return nodes
}

// Verify verifies the VAR nodes are correctly calculated and that the VAR extends the previous VAR if provided.
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

	// verify the node ids are correctly calculated (no nodes in shard is considered valid)
	for i, n := range v.Nodes {
		if err := n.Verify(); err != nil {
			return fmt.Errorf("invalid node at idx %d: %w", i, err)
		}
	}
	return nil
}

// Verify verifies the node id is derived from the auth key
func (v *NodeInfo) Verify() error {
	authKey, err := libp2pcrypto.UnmarshalSecp256k1PublicKey(v.AuthKey)
	if err != nil {
		return fmt.Errorf("unmarshal auth key: %w", err)
	}
	nodeID, err := peer.IDFromPublicKey(authKey)
	if err != nil {
		return err
	}
	if nodeID.String() != v.NodeID {
		return errors.New("node id is not hash of auth key")
	}
	if _, err := crypto.NewVerifierSecp256k1(v.SignKey); err != nil {
		return fmt.Errorf("invalid sign key for node %s: %w", v.NodeID, err)
	}
	return nil
}
