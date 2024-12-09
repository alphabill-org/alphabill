package rpc

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"time"

	"github.com/multiformats/go-multiaddr"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
)

type (
	AdminAPI struct {
		node partitionNode
		name string
		self *network.Peer
		log  *slog.Logger

		updMetrics func(ctx context.Context, method string, start time.Time, apiErr error)
	}

	NodeInfoResponse struct {
		NetworkID           types.NetworkID       `json:"networkId"`   // hex encoded network identifier
		PartitionID         types.PartitionID     `json:"partitionId"` // hex encoded partition identifier
		PartitionTypeID     types.PartitionTypeID `json:"partitionTypeId"` // hex encoded partition identifier
		PermissionedMode    bool                  `json:"permissionedMode"`
		FeelessMode         bool                  `json:"feelessMode"`
		Self                PeerInfo              `json:"self"` // information about this peer
		BootstrapNodes      []PeerInfo            `json:"bootstrapNodes"`
		RootValidators      []PeerInfo            `json:"rootValidators"`
		PartitionValidators []PeerInfo            `json:"partitionValidators"`
		OpenConnections     []PeerInfo            `json:"openConnections"` // all libp2p connections to other peers in the network
	}

	PeerInfo struct {
		NodeID     string                `json:"nodeId"`
		Addresses  []multiaddr.Multiaddr `json:"addresses"`
	}
)

func NewAdminAPI(node partitionNode, self *network.Peer, obs Observability) *AdminAPI {
	return &AdminAPI{
		node:       node,
		self:       self,
		log:        obs.Logger(),
		updMetrics: metricsUpdater(obs.Meter(metricsScopeJRPCAPI), node, obs.Logger()),
	}
}

// GetNodeInfo returns information about the node.
func (s *AdminAPI) GetNodeInfo(ctx context.Context) (_ *NodeInfoResponse, retErr error) {
	defer func(start time.Time) { s.updMetrics(ctx, "getNodeInfo", start, retErr) }(time.Now())
	return &NodeInfoResponse{
		NetworkID:        s.node.NetworkID(),
		PartitionID:      s.node.PartitionID(),
		PartitionTypeID:  s.node.PartitionTypeID(),
		PermissionedMode: s.node.IsPermissionedMode(),
		FeelessMode:      s.node.IsFeelessMode(),
		Self: PeerInfo{
			NodeID: s.self.ID().String(),
			Addresses:  s.self.MultiAddresses(),
		},
		BootstrapNodes:      getBootstrapNodes(s.self),
		RootValidators:      getRootValidators(s.self, s.log),
		PartitionValidators: getPartitionValidators(s.node, s.self),
		OpenConnections:     getOpenConnections(s.self),
	}, nil
}

func getPartitionValidators(node partitionNode, self *network.Peer) []PeerInfo {
	validators := node.Validators()
	peers := make([]PeerInfo, len(validators))
	peerStore := self.Network().Peerstore()
	for i, v := range validators {
		peers[i] = PeerInfo{
			NodeID: v.String(),
			Addresses:  peerStore.PeerInfo(v).Addrs,
		}
	}
	return peers
}

func getOpenConnections(self *network.Peer) []PeerInfo {
	connections := self.Network().Conns()
	peers := make([]PeerInfo, len(connections))
	for i, connection := range connections {
		peers[i] = PeerInfo{
			NodeID: connection.RemotePeer().String(),
			Addresses:  []multiaddr.Multiaddr{connection.RemoteMultiaddr()},
		}
	}
	return peers
}

func getRootValidators(self *network.Peer, log *slog.Logger) []PeerInfo {
	var peers []PeerInfo
	peerStore := self.Network().Peerstore()
	ids := peerStore.Peers()
	for _, id := range ids {
		protocols, err := peerStore.SupportsProtocols(id, network.ProtocolBlockCertification)
		if err != nil {
			log.Warn("failed to query peer store", logger.Error(err))
			continue
		}
		if slices.Contains(protocols, network.ProtocolBlockCertification) {
			peers = append(peers, PeerInfo{
				NodeID: id.String(),
				Addresses:  peerStore.PeerInfo(id).Addrs,
			})
		}
	}
	return peers
}

func getBootstrapNodes(self *network.Peer) []PeerInfo {
	bootstrapPeers := self.Configuration().BootstrapPeers
	infos := make([]PeerInfo, len(bootstrapPeers))
	for i, p := range bootstrapPeers {
		infos[i] = PeerInfo{NodeID: p.ID.String(), Addresses: p.Addrs}
	}
	return infos
}

func (pi *PeerInfo) UnmarshalJSON(data []byte) error {
	var d map[string]interface{}
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	pi.NodeID, _ = d["identifier"].(string)
	addrs := d["addresses"].([]interface{})
	for _, addr := range addrs {
		multiAddr, err := multiaddr.NewMultiaddr(addr.(string))
		if err != nil {
			return err
		}
		pi.Addresses = append(pi.Addresses, multiAddr)
	}
	return nil
}
