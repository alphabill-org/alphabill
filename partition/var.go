package partition

import (
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
		Nodes       []ValidatorInfo   `json:"nodes"`
	}

	ValidatorInfo struct {
		_       struct{}  `cbor:",toarray"`
		NodeID  string    `json:"nodeId"`  // libp2p node id (hash of libp2p encryption public key)
		AuthKey hex.Bytes `json:"authKey"` // libp2p encryption public key
		SigKey  hex.Bytes `json:"sigKey"`  // alphabill signing public key
	}
)

func newVARFromGenesis(genesis *genesis.PartitionGenesis) *ValidatorAssignmentRecord {
	if genesis == nil {
		return nil
	}

	validators := make([]ValidatorInfo, len(genesis.Keys))
	for i, k := range genesis.Keys {
		validators[i] = ValidatorInfo{
			NodeID:  k.NodeIdentifier,
			AuthKey: k.EncryptionPublicKey,
			SigKey:  k.SigningPublicKey,
		}
	}

	return &ValidatorAssignmentRecord{
		NetworkID:   genesis.PartitionDescription.NetworkIdentifier,
		PartitionID: genesis.PartitionDescription.PartitionIdentifier,
		EpochNumber: genesis.Certificate.InputRecord.Epoch,
		RoundNumber: genesis.Certificate.GetRoundNumber(),
		Nodes:       validators,
	}
}

// Verify verifies the current VAR extends the previous VAR.
func (v *ValidatorAssignmentRecord) Verify(prev *ValidatorAssignmentRecord) error {
	if prev != nil {
		if v.NetworkID != prev.NetworkID {
			return fmt.Errorf("invalid network id, current %d previous %v", v.NetworkID, prev.NetworkID)
		}
		if v.PartitionID != prev.PartitionID {
			return fmt.Errorf("invalid partition id, current %d previous %v", v.NetworkID, prev.NetworkID)
		}
		if !v.ShardID.Equal(prev.ShardID) {
			return fmt.Errorf("invalid shard id, current %x previous %x", v.ShardID, prev.ShardID)
		}
		if v.EpochNumber != prev.EpochNumber+1 {
			return fmt.Errorf("invalid epoch number, current %d previous %d", v.EpochNumber, prev.EpochNumber)
		}
		if v.RoundNumber <= prev.RoundNumber {
			return fmt.Errorf("invalid shard round number, current %d previous %d", v.RoundNumber, prev.RoundNumber)
		}
	}
	// verify the node ids are correctly calculated (no nodes in shard is considered valid)
	for i, n := range v.Nodes {
		if err := n.Verify(); err != nil {
			return fmt.Errorf("invalid node idx %d: %w", i, err)
		}
	}
	return nil
}

// Verify verifies the node id is derived from the auth key
func (v *ValidatorInfo) Verify() error {
	authKey, err := libp2pcrypto.UnmarshalSecp256k1PublicKey(v.AuthKey)
	if err != nil {
		return fmt.Errorf("unmarshal auth key: %w", err)
	}
	nodeID, err := peer.IDFromPublicKey(authKey)
	if err != nil {
		return err
	}
	if nodeID.String() != v.NodeID {
		return fmt.Errorf("invalid node ID: %s", v.NodeID)
	}
	if _, err := crypto.NewVerifierSecp256k1(v.SigKey); err != nil {
		return fmt.Errorf("invalid sign key for node %s: %w", v.NodeID, err)
	}
	return nil
}
