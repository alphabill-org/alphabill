package network

import (
	"context"
	"crypto/rand"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"github.com/hashicorp/go-multierror"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
	"go.uber.org/multierr"
)

const (
	ErrStringInvalidPrivateKey = "invalid private key"
	ErrStringInvalidPublicKey  = "invalid public key"

	defaultAddress    = "/ip4/0.0.0.0/tcp/0"
	p2pProtocolPrefix = "/p2p/"
)

var logger = log.CreateForPackage()

type (

	// PeerConfiguration includes single peer configuration values.
	PeerConfiguration struct {
		Address        string       // address to listen for incoming connections. Uses libp2p multiaddress format.
		KeyPair        *PeerKeyPair // keypair for the peer.
		BootstrapPeers []*PeerInfo  // a list of bootstrap peers.
	}

	// PeerInfo contains peer.ID and address.
	PeerInfo struct {
		ID      string // id of the peer
		Address string // address of the peer
	}

	// PeerKeyPair contains node's public and private key.
	PeerKeyPair struct {
		Pub  []byte
		Priv []byte
	}

	// Peer represents a single node in p2p network. It is a wrapper around the libp2p host.Host.
	Peer struct {
		host host.Host
		dht  *dht.IpfsDHT
		conf *PeerConfiguration
	}
)

// NewPeer constructs a new peer node with given configuration. If no peer key is provided, it generates a random
// Secp256k1 key-pair and derives a new identity from it. If no transport and listen addresses are provided, the node
// listens to the multiaddresses "/ip4/0.0.0.0/tcp/0".
func NewPeer(ctx context.Context, conf *PeerConfiguration) (*Peer, error) {
	// keys
	privateKey, _, err := readOrGenerateKeyPair(conf)
	if err != nil {
		return nil, err
	}

	// address
	address := defaultAddress
	if conf != nil && conf.Address != "" {
		address = conf.Address
	}

	// create a new libp2p Host
	h, err := libp2p.New(libp2p.ListenAddrStrings(address), libp2p.Identity(privateKey))
	if err != nil {
		return nil, err
	}
	p := &Peer{host: h, conf: conf}
	logger.Info("Host ID=%v, addresses=%v", h.ID(), h.Addrs())

	// create DHT
	bootstrapNodes, err := getBootstrapNodes(conf)
	if err != nil {
		return nil, err
	}
	dht, err := newDHT(ctx, h, bootstrapNodes)
	if err != nil {
		logger.Error("DHT init failed: %v. Closing peer.", err)
		closeErr := p.Close()
		if closeErr != nil {
			err = multierror.Append(err, closeErr)
		}
		return nil, err
	}
	p.dht = dht

	return p, nil
}

// ID returns the identifier associated with this Peer.
func (p *Peer) ID() peer.ID {
	return p.host.ID()
}

// MultiAddresses the address associated with this Peer.
func (p *Peer) MultiAddresses() []ma.Multiaddr {
	return p.host.Addrs()
}

// RegisterProtocolHandler sets the protocol stream handler for given protocol.
func (p *Peer) RegisterProtocolHandler(protocolID string, handler network.StreamHandler) {
	p.host.SetStreamHandler(protocol.ID(protocolID), handler)
}

// RemoveProtocolHandler removes the given protocol handler.
func (p *Peer) RemoveProtocolHandler(protocolID string) {
	p.host.RemoveStreamHandler(protocol.ID(protocolID))
}

// CreateStream opens a new stream to given peer p, and writes a libp2p protocol header with given ProtocolID.
func (p *Peer) CreateStream(ctx context.Context, peerID peer.ID, protocolID string) (network.Stream, error) {
	return p.host.NewStream(ctx, peerID, protocol.ID(protocolID))
}

func (p *Peer) RoutingTableSize() int {
	return len(p.dht.RoutingTable().ListPeers())
}

// Close shuts down the libp2p host and related services.
func (p *Peer) Close() (res error) {
	logger.Info("Closing peer")
	// close dht
	if p.dht != nil {
		err := p.dht.Close()
		if err != nil {
			res = multierr.Append(res, err)
		}
	}

	// close libp2p host
	logger.Debug("Stopping libp2p node")
	if err := p.host.Close(); err != nil {
		res = multierror.Append(res, err)
	}
	logger.Debug("Closing peer store")
	// to prevent peerstore go routine leak (https://github.com/libp2p/go-libp2p/issues/718)
	if err := p.host.Peerstore().Close(); err != nil {
		res = multierror.Append(res, err)
	}
	return
}

func readOrGenerateKeyPair(conf *PeerConfiguration) (privateKey crypto.PrivKey, publicKey crypto.PubKey, err error) {
	if conf == nil || conf.KeyPair == nil {
		logger.Warning("Peer key not found! Generating a new random key.")
		privateKey, publicKey, err = crypto.GenerateEd25519Key(rand.Reader)
	} else {
		privateKey, err = crypto.UnmarshalPrivateKey(conf.KeyPair.Priv)
		if err != nil {
			err = errors.Wrap(err, ErrStringInvalidPrivateKey)
			return
		}
		publicKey, err = crypto.UnmarshalPublicKey(conf.KeyPair.Pub)
		if err != nil {
			err = errors.Wrap(err, ErrStringInvalidPublicKey)
			return
		}
	}
	return
}

func getBootstrapNodes(conf *PeerConfiguration) (dht.Option, error) {
	if conf == nil || len(conf.BootstrapPeers) == 0 {
		return nil, nil
	}
	peers := conf.BootstrapPeers
	peerInfos := make([]peer.AddrInfo, len(peers))
	for i, p := range peers {
		maddrPeer, err := ma.NewMultiaddr(p2pProtocolPrefix + p.ID)
		if err != nil {
			return nil, err
		}
		maddrTpt := ma.StringCast(p.Address)
		maddrFull := maddrTpt.Encapsulate(maddrPeer)

		addInfo, err := peer.AddrInfoFromP2pAddr(maddrFull)
		if err != nil {
			return nil, err
		}
		peerInfos[i] = *addInfo
	}
	return dht.BootstrapPeers(peerInfos...), nil
}
