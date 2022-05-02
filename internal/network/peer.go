package network

import (
	"context"
	"crypto/rand"
	mrand "math/rand"
	"time"

	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/libp2p/go-libp2p-peerstore/pstoremem"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	log "gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"github.com/hashicorp/go-multierror"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	ErrStringInvalidPrivateKey = "invalid private key"
	ErrStringInvalidPublicKey  = "invalid public key"

	defaultAddress = "/ip4/0.0.0.0/tcp/0"
)

var (
	ErrPeerConfigurationIsNil = errors.New("peer configuration is nil")

	logger = log.CreateForPackage()
)

type (

	// PeerConfiguration includes single peer configuration values.
	PeerConfiguration struct {
		Address         string       // address to listen for incoming connections. Uses libp2p multiaddress format.
		KeyPair         *PeerKeyPair // keypair for the peer.
		PersistentPeers []*PeerInfo  // a list of known peers (in case of partition node this list must contain all validators).
	}

	// PeerInfo contains a public key and address.
	PeerInfo struct {
		Address   string // address of the peer
		PublicKey []byte // public key of the peer
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
	}
)

// NewPeer constructs a new peer node with given configuration. If no peer key is provided, it generates a random
// Secp256k1 key-pair and derives a new identity from it. If no transport and listen addresses are provided, the node
// listens to the multiaddresses "/ip4/0.0.0.0/tcp/0".
func NewPeer(conf *PeerConfiguration) (*Peer, error) {
	if conf == nil {
		return nil, ErrPeerConfigurationIsNil
	}
	// keys
	privateKey, publicKey, err := readOrGenerateKeyPair(conf)
	if err != nil {
		return nil, err
	}

	// address
	address := defaultAddress
	if conf.Address != "" {
		address = conf.Address
	}
	// node identifier
	id, err := peer.IDFromPublicKey(publicKey)
	if err != nil {
		return nil, err
	}

	// create a new peerstore
	peerStore, err := newPeerStore(conf.PersistentPeers, id)
	if err != nil {
		return nil, err
	}

	// create a new libp2p Host
	h, err := libp2p.New(libp2p.ListenAddrStrings(address), libp2p.Identity(privateKey), libp2p.Peerstore(peerStore))
	if err != nil {
		return nil, err
	}
	p := &Peer{host: h, conf: conf}
	logger.Debug("Host ID=%v, addresses=%v", h.ID(), h.Addrs())

	return p, nil
}

// ID returns the identifier associated with this Peer.
func (p *Peer) ID() peer.ID {
	return p.host.ID()
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

// Configuration returns peer configuration
func (p *Peer) Configuration() *PeerConfiguration {
	return p.conf
}

// Close shuts down the libp2p host and related services.
func (p *Peer) Close() (res error) {
	logger.Info("Closing peer")

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

// GetRandomPeerID returns a random peer.ID from the peerstore.
func (p *Peer) GetRandomPeerID() peer.ID {
	networkPeers := p.Network().Peerstore().Peers()

	var peers []peer.ID
	for _, id := range networkPeers {
		if id != p.ID() {
			peers = append(peers, id)
		}
	}

	mrand.Seed(time.Now().UnixNano())
	index := mrand.Intn(len(peers))
	return peers[index]
}

func (pi *PeerInfo) GetID() (peer.ID, error) {
	pub, err := crypto.UnmarshalPublicKey(pi.PublicKey)
	if err != nil {
		return "", err
	}
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return "", err
	}
	return id, nil
}

func newPeerStore(peers []*PeerInfo, self peer.ID) (peerstore.Peerstore, error) {
	peerStore, err := pstoremem.NewPeerstore()
	if err != nil {
		return nil, err
	}

	for _, p := range peers {
		id, err := p.GetID()
		if err != nil {
			return nil, err
		}

		if id == self {
			// ignore itself
			continue
		}

		addr, err := ma.NewMultiaddr(p.Address)
		if err != nil {
			return nil, err
		}

		peerStore.AddAddr(id, addr, peerstore.PermanentAddrTTL)

	}
	return peerStore, nil
}

func readOrGenerateKeyPair(conf *PeerConfiguration) (privateKey crypto.PrivKey, publicKey crypto.PubKey, err error) {
	if conf.KeyPair == nil {
		logger.Warning("Peer key not found! Generating a new random key.")
		privateKey, publicKey, err = crypto.GenerateEd25519Key(rand.Reader)
	} else {
		privateKey, err = crypto.UnmarshalPrivateKey(conf.KeyPair.PrivateKey)
		if err != nil {
			err = errors.Wrap(err, ErrStringInvalidPrivateKey)
			return
		}
		publicKey, err = crypto.UnmarshalPublicKey(conf.KeyPair.PublicKey)
		if err != nil {
			err = errors.Wrap(err, ErrStringInvalidPublicKey)
			return
		}
	}
	return
}
