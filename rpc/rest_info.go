package rpc

import (
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"slices"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network"
	"github.com/gorilla/mux"
	"github.com/multiformats/go-multiaddr"
)

type (
	infoResponse struct {
		SystemID            string     `json:"system_id"` // hex encoded system identifier
		Name                string     `json:"name"`      // one of [money node | tokens node | evm node]
		Self                peerInfo   `json:"self"`      // information about this peer
		BootstrapNodes      []peerInfo `json:"bootstrap_nodes"`
		RootValidators      []peerInfo `json:"root_validators"`
		PartitionValidators []peerInfo `json:"partition_validators"`
		OpenConnections     []peerInfo `json:"open_connections"` // all libp2p connections to other peers in the network

	}

	peerInfo struct {
		Identifier string                `json:"identifier"`
		Addresses  []multiaddr.Multiaddr `json:"addresses"`
	}
)

func InfoEndpoints(node partitionNode, name string, self *network.Peer, log *slog.Logger) RegistrarFunc {
	return func(r *mux.Router) {
		r.HandleFunc("/info", infoHandler(node, name, self, log)).Methods(http.MethodGet, http.MethodOptions)
	}
}

func infoHandler(node partitionNode, name string, self *network.Peer, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		i := infoResponse{
			SystemID: hex.EncodeToString(node.SystemIdentifier()),
			Name:     name,
			Self: peerInfo{
				Identifier: self.ID().String(),
				Addresses:  self.MultiAddresses(),
			},
			BootstrapNodes:      getBootstrapNodes(self),
			RootValidators:      getRootValidators(self, log),
			PartitionValidators: getPartitionValidators(self),
			OpenConnections:     getOpenConnections(self),
		}
		w.Header().Set(headerContentType, applicationJson)
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(i); err != nil {
			log.WarnContext(r.Context(), "failed to write info message", logger.Error(err))
		}
	}

}

func getPartitionValidators(self *network.Peer) []peerInfo {
	validators := self.Validators()
	peers := make([]peerInfo, len(validators))
	peerStore := self.Network().Peerstore()
	for i, v := range validators {
		peers[i] = peerInfo{
			Identifier: v.String(),
			Addresses:  peerStore.PeerInfo(v).Addrs,
		}
	}
	return peers
}

func getOpenConnections(self *network.Peer) []peerInfo {
	connections := self.Network().Conns()
	peers := make([]peerInfo, len(connections))
	for i, connection := range connections {
		peers[i] = peerInfo{
			Identifier: connection.RemotePeer().String(),
			Addresses:  []multiaddr.Multiaddr{connection.RemoteMultiaddr()},
		}
	}
	return peers
}

func getRootValidators(self *network.Peer, log *slog.Logger) []peerInfo {
	var peers []peerInfo
	peerStore := self.Network().Peerstore()
	ids := peerStore.Peers()
	for _, id := range ids {
		protocols, err := peerStore.SupportsProtocols(id, network.ProtocolBlockCertification)
		if err != nil {
			log.Warn("failed to query peer store", logger.Error(err))
			continue
		}
		if slices.Contains(protocols, network.ProtocolBlockCertification) {
			peers = append(peers, peerInfo{
				Identifier: id.String(),
				Addresses:  peerStore.PeerInfo(id).Addrs,
			})
		}
	}
	return peers
}

func getBootstrapNodes(self *network.Peer) []peerInfo {
	bootstrapPeers := self.Configuration().BootstrapPeers
	infos := make([]peerInfo, len(bootstrapPeers))
	for i, p := range bootstrapPeers {
		infos[i] = peerInfo{Identifier: p.ID.String(), Addresses: p.Addrs}
	}
	return infos
}

func (pi *peerInfo) UnmarshalJSON(data []byte) error {
	var d map[string]interface{}
	if err := json.Unmarshal(data, &d); err != nil {
		return err
	}

	pi.Identifier, _ = d["identifier"].(string)
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
