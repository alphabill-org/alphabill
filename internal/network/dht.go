package network

import (
	"context"
	"github.com/libp2p/go-libp2p-core/host"
	dht "github.com/libp2p/go-libp2p-kad-dht"
)

// AlphaBillProtocolPrefix protocol prefix is attached to libp2p kad protocol. This allows to use
// "/ab/kad/1.0.0" instead of "/ipfs/kad/1.0.0".
const AlphaBillProtocolPrefix = "/ab"

// newDHT creates a new DHT with the specified host and options.
func newDHT(ctx context.Context, host host.Host, options ...dht.Option) (*dht.IpfsDHT, error) {
	opts := []dht.Option{
		dht.ProtocolPrefix(AlphaBillProtocolPrefix),
		dht.Mode(dht.ModeServer),
	}
	if options != nil && len(options) > 0 {
		for _, option := range options {
			if option != nil {
				opts = append(opts, option)
			}
		}
	}

	kademliaDHT, err := dht.New(ctx, host, opts...)
	logger.Info("DHT initialized. Address=%v; host ID=%v", kademliaDHT.Host().Addrs(), kademliaDHT.Host().ID())
	if err != nil {
		return nil, err
	}
	if err = kademliaDHT.Bootstrap(ctx); err != nil {
		return nil, err
	}
	return kademliaDHT, nil
}
