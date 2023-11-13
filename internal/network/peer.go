package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	mrand "math/rand"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	libp2pprotocol "github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	defaultAddress    = "/ip4/0.0.0.0/tcp/0"
	dhtProtocolPrefix = "/ab/dht/0.1.0"
)

var (
	ErrPeerConfigurationIsNil = errors.New("peer configuration is nil")
)

type (
	// PeerConfiguration includes single peer configuration values.
	PeerConfiguration struct {
		ID             peer.ID         // peer identifier derived from the KeyPair.PublicKey.
		Address        string          // address to listen for incoming connections. Uses libp2p multiaddress format.
		KeyPair        *PeerKeyPair    // keypair for the peer.
		BootstrapPeers []peer.AddrInfo // a list of seed peers to connect to.
		Validators     []peer.ID       // a list of known peers (in case of partition node this list must contain all validators).
	}

	// PeerKeyPair contains node's public and private key.
	PeerKeyPair struct {
		PublicKey  []byte
		PrivateKey []byte
	}

	// Peer represents a single node in p2p network. It is a wrapper around the libp2p host.Host.
	Peer struct {
		host       host.Host
		conf       *PeerConfiguration
		validators []peer.ID
		dht        *dht.IpfsDHT
	}
)

// NewPeer constructs a new peer node with given configuration. If no peer key is provided, it generates a random
// Secp256k1 key-pair and derives a new identity from it. If no transport and listen addresses are provided, the node
// listens to the multiaddresses "/ip4/0.0.0.0/tcp/0".
func NewPeer(ctx context.Context, conf *PeerConfiguration, log *slog.Logger, prom prometheus.Registerer) (*Peer, error) {
	if conf == nil {
		return nil, ErrPeerConfigurationIsNil
	}
	// keys
	privateKey, _, err := readKeyPair(conf, log)
	if err != nil {
		return nil, err
	}

	// address
	address := defaultAddress
	if conf.Address != "" {
		address = conf.Address
	}

	// create a new peerstore
	peerStore, err := newPeerStore()
	if err != nil {
		return nil, err
	}

	opts := []config.Option{
		libp2p.ListenAddrStrings(address),
		libp2p.Identity(privateKey),
		libp2p.Peerstore(peerStore),
		libp2p.Ping(true), // make sure ping service is enabled
	}
	if prom != nil {
		opts = append(opts, libp2p.PrometheusRegisterer(prom))
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}

	kademliaDHT, err := newDHT(ctx, h, conf.BootstrapPeers, dht.ModeServer)
	if err != nil {
		if e := h.Close(); e != nil {
			err = errors.Join(err, fmt.Errorf("closing libp2p host: %w", err))
		}
		return nil, err
	}
	log.DebugContext(ctx, fmt.Sprintf("addresses=%v; bootstrap peers=%v", h.Addrs(), conf.BootstrapPeers), logger.NodeID(h.ID()))

	p := &Peer{host: h, conf: conf, dht: kademliaDHT, validators: conf.Validators}
	return p, nil
}

// ID returns the identifier associated with this Peer.
func (p *Peer) ID() peer.ID {
	return p.host.ID()
}

// String returns short representation of node id
func (p *Peer) String() string {
	id := p.ID().String()
	if len(id) <= 10 {
		return fmt.Sprintf("NodeID:%s", id)
	}
	return fmt.Sprintf("NodeID:%s*%s", id[:2], id[len(id)-6:])
}

func (p *Peer) Validators() []peer.ID {
	return p.validators
}

func (p *Peer) FilterValidators(exclude peer.ID) []peer.ID {
	var validatorIdentifiers []peer.ID
	for _, v := range p.validators {
		if v != exclude {
			validatorIdentifiers = append(validatorIdentifiers, v)
		}
	}
	return validatorIdentifiers
}

// PublicKey returns the public key of the peer.
func (p *Peer) PublicKey() (crypto.PubKey, error) {
	return p.ID().ExtractPublicKey()
}

// MultiAddresses the address associated with this Peer.
func (p *Peer) MultiAddresses() []ma.Multiaddr {
	return p.host.Addrs()
}

// Network returns the Network of the Peer.
func (p *Peer) Network() network.Network {
	return p.host.Network()
}

func (p *Peer) BootstrapNodes() []peer.AddrInfo {
	return p.Configuration().BootstrapPeers
}

// RegisterProtocolHandler sets the protocol stream handler for given protocol.
func (p *Peer) RegisterProtocolHandler(protocolID string, handler network.StreamHandler) {
	p.host.SetStreamHandler(libp2pprotocol.ID(protocolID), handler)
}

// RemoveProtocolHandler removes the given protocol handler.
func (p *Peer) RemoveProtocolHandler(protocolID string) {
	p.host.RemoveStreamHandler(libp2pprotocol.ID(protocolID))
}

// CreateStream opens a new stream to given peer p, and writes a libp2p protocol header with given ProtocolID.
func (p *Peer) CreateStream(ctx context.Context, peerID peer.ID, protocolID string) (network.Stream, error) {
	return p.host.NewStream(ctx, peerID, libp2pprotocol.ID(protocolID))
}

// Configuration returns peer configuration
func (p *Peer) Configuration() *PeerConfiguration {
	return p.conf
}

// Close shuts down the libp2p host and related services.
func (p *Peer) Close() error {
	var err error
	if cerr := p.dht.Close(); cerr != nil {
		err = fmt.Errorf("closing the DHT: %w", cerr)
	}
	// close libp2p host
	if cerr := p.host.Close(); cerr != nil {
		err = errors.Join(err, fmt.Errorf("closing the host: %w", cerr))
	}
	return err
}

// GetRandomPeerID returns a random peer.ID from the peerstore.
func (p *Peer) GetRandomPeerID() peer.ID {
	networkPeers := p.Network().Peerstore().Peers()

	var peers []peer.ID
	for _, id := range networkPeers {
		if id != p.ID() {
			peers = append(peers, id)
		}
	}

	// #nosec G404
	index := mrand.Intn(len(peers))
	return peers[index]
}

func NewPeerConfiguration(
	addr string,
	keyPair *PeerKeyPair,
	bootstrapPeers []peer.AddrInfo,
	validators []peer.ID) (*PeerConfiguration, error) {

	if keyPair == nil {
		return nil, fmt.Errorf("missing key pair")
	}

	peerID, err := NodeIDFromPublicKeyBytes(keyPair.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid key pair: %w", err)
	}

	return &PeerConfiguration{
		ID:             peerID,
		Address:        addr,
		KeyPair:        keyPair,
		BootstrapPeers: bootstrapPeers,
		Validators:     validators,
	}, nil
}

func NodeIDFromPublicKeyBytes(pubKey []byte) (peer.ID, error) {
	pub, err := crypto.UnmarshalSecp256k1PublicKey(pubKey)
	if err != nil {
		return "", err
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return "", err
	}
	return id, nil
}

func newDHT(ctx context.Context, h host.Host, bootstrapPeers []peer.AddrInfo, opt dht.ModeOpt) (*dht.IpfsDHT, error) {
	kdht, err := dht.New(ctx, h, dht.ProtocolPrefix(dhtProtocolPrefix), dht.BootstrapPeers(bootstrapPeers...), dht.Mode(opt))
	if err != nil {
		return nil, fmt.Errorf("creating DHT: %w", err)
	}
	if err = kdht.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("bootstrapping DHT: %w", err)
	}
	return kdht, nil
}

func newPeerStore() (peerstore.Peerstore, error) {
	peerStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, err
	}
	return peerStore, nil
}

func readKeyPair(conf *PeerConfiguration, log *slog.Logger) (privateKey crypto.PrivKey, publicKey crypto.PubKey, err error) {
	if conf.KeyPair == nil {
		return nil, nil, fmt.Errorf("missing peer key")
	}

	privateKey, err = crypto.UnmarshalSecp256k1PrivateKey(conf.KeyPair.PrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid private key: %w", err)
	}
	publicKey, err = crypto.UnmarshalSecp256k1PublicKey(conf.KeyPair.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid public key: %w", err)
	}
	return privateKey, publicKey, nil
}
