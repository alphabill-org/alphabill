package network

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/libp2p/go-libp2p"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p/config"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	libp2pprotocol "github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-libp2p/core/routing"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"
	"github.com/libp2p/go-libp2p/p2p/host/peerstore/pstoremem"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/prometheus/client_golang/prometheus"
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
		AnnounceAddrs  []ma.Multiaddr  // callback addresses to announce to other peers, if specified then overwrites any and all default listen addresses
		KeyPair        *PeerKeyPair    // keypair for the peer.
		BootstrapPeers []peer.AddrInfo // a list of seed peers to connect to.
	}

	// PeerKeyPair contains node's public and private key.
	PeerKeyPair struct {
		PublicKey  []byte
		PrivateKey []byte
	}

	// Peer represents a single node in p2p network. It is a wrapper around the libp2p host.Host.
	Peer struct {
		host host.Host
		conf *PeerConfiguration
		dht  *dht.IpfsDHT
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
	privateKey, _, err := readKeyPair(conf)
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

	var kademliaDHT *dht.IpfsDHT

	opts := []config.Option{
		libp2p.ListenAddrStrings(address),
		libp2p.Identity(privateKey),
		libp2p.Peerstore(peerStore),
		// Let this host use the DHT to find other hosts
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			kademliaDHT, err = newDHT(ctx, h, conf.BootstrapPeers, dht.ModeServer, log)
			return kademliaDHT, err
		}),

		libp2p.Ping(true), // make sure ping service is enabled
	}
	if prom != nil {
		opts = append(opts, libp2p.PrometheusRegisterer(prom))
	}
	if len(conf.AnnounceAddrs) > 0 {
		addrsFactory := libp2p.AddrsFactory(func(_ []ma.Multiaddr) []ma.Multiaddr {
			// completely overwrite default announce addresses with provided values
			// and make a defensive copy, consumers can modify the slice elements
			res := make([]ma.Multiaddr, len(conf.AnnounceAddrs))
			copy(res, conf.AnnounceAddrs)
			return res
		})
		opts = append(opts, addrsFactory)
	}
	h, err := libp2p.New(opts...)
	if err != nil {
		return nil, err
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, fmt.Errorf("bootstrapping DHT: %w", err)
	}
	log.DebugContext(ctx, fmt.Sprintf("addresses=%v; bootstrap peers=%v", h.Addrs(), conf.BootstrapPeers))

	return &Peer{
		host: h,
		conf: conf,
		dht:  kademliaDHT,
	}, nil
}

// This code is borrowed from the go-ipfs bootstrap process
func (p *Peer) BootstrapConnect(ctx context.Context, log *slog.Logger) error {
	if len(p.conf.BootstrapPeers) == 0 {
		return nil
	}

	errs := make(chan error, len(p.conf.BootstrapPeers))
	var wg sync.WaitGroup
	for _, peerAddr := range p.conf.BootstrapPeers {
		// performed asynchronously because when performed synchronously, if
		// one `Connect` call hangs, subsequent calls are more likely to
		// fail/abort due to an expiring context.
		// Also, performed asynchronously for dial speed.
		wg.Add(1)
		go func(peerAddr peer.AddrInfo) {
			defer wg.Done()
			p.host.Peerstore().AddAddrs(peerAddr.ID, peerAddr.Addrs, peerstore.PermanentAddrTTL)
			if err := p.host.Connect(ctx, peerAddr); err != nil {
				log.WarnContext(ctx, fmt.Sprintf("Bootstrap dial %s to %s failed: %s", p.host.ID(), peerAddr.ID, err))
				errs <- err
				return
			}
			log.DebugContext(ctx, fmt.Sprintf("Bootstrap dial %s to %s: success", p.host.ID(), peerAddr.ID))
		}(peerAddr)
	}
	wg.Wait()

	// our failure condition is when no connection attempt succeeded.
	// So drain the errs channel, counting the results.
	close(errs)
	count := 0
	var allErr error
	for err := range errs {
		if err != nil {
			count++
			allErr = errors.Join(allErr, err)
		}
	}
	if count == len(p.conf.BootstrapPeers) {
		return fmt.Errorf("failed to bootstrap: %w", allErr)
	}
	return p.dht.Bootstrap(ctx)
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

// MultiAddresses the address associated with this Peer.
func (p *Peer) MultiAddresses() []ma.Multiaddr {
	return p.host.Addrs()
}

// Network returns the Network of the Peer.
func (p *Peer) Network() network.Network {
	return p.host.Network()
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

func (p *Peer) Advertise(ctx context.Context, topic string) error {
	routingDiscovery := drouting.NewRoutingDiscovery(p.dht)
	_, err := routingDiscovery.Advertise(ctx, topic)
	return err
}

func (p *Peer) Discover(ctx context.Context, topic string) (<-chan peer.AddrInfo, error) {
	routingDiscovery := drouting.NewRoutingDiscovery(p.dht)
	return routingDiscovery.FindPeers(ctx, topic)
}

func NewPeerConfiguration(
	addr string,
	announceAddrs []string,
	keyPair *PeerKeyPair,
	bootstrapPeers []peer.AddrInfo) (*PeerConfiguration, error) {

	if keyPair == nil {
		return nil, fmt.Errorf("missing key pair")
	}

	peerID, err := NodeIDFromPublicKeyBytes(keyPair.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("invalid key pair: %w", err)
	}

	var announceMultiAddrs []ma.Multiaddr
	for _, announceAddr := range announceAddrs {
		announceMultiAddr, err := ma.NewMultiaddr(announceAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to convert announce address '%s' to libp2p multiaddress format: %w", announceAddr, err)
		}
		announceMultiAddrs = append(announceMultiAddrs, announceMultiAddr)
	}

	return &PeerConfiguration{
		ID:             peerID,
		Address:        addr,
		AnnounceAddrs:  announceMultiAddrs,
		KeyPair:        keyPair,
		BootstrapPeers: bootstrapPeers,
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

func newDHT(ctx context.Context, h host.Host, bootstrapPeers []peer.AddrInfo, opt dht.ModeOpt, log *slog.Logger) (*dht.IpfsDHT, error) {
	kdht, err := dht.New(ctx, h, dht.ProtocolPrefix(dhtProtocolPrefix), dht.BootstrapPeers(bootstrapPeers...), dht.Mode(opt))
	if err != nil {
		return nil, fmt.Errorf("creating DHT: %w", err)
	}
	routingTable := kdht.RoutingTable()
	peerRemovedCb := routingTable.PeerRemoved
	peerAddedCb := routingTable.PeerAdded
	routingTable.PeerRemoved = func(pid peer.ID) {
		peerRemovedCb(pid)
		log.DebugContext(ctx, fmt.Sprintf("peer %s removed from routing table", pid.String()))
		// meter routing table size? -> decrease
	}
	routingTable.PeerAdded = func(pid peer.ID) {
		peerAddedCb(pid)
		log.DebugContext(ctx, fmt.Sprintf("peer %s added to routing table", pid.String()))
		// meter routing table size? -> increase?
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

func readKeyPair(conf *PeerConfiguration) (privateKey crypto.PrivKey, publicKey crypto.PubKey, err error) {
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
