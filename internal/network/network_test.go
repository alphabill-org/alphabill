package network

/*
func TestNewNetwork(t *testing.T) {
	conf := &PeerConfiguration{}
	conf.Address = "/ip4/127.0.0.1/tcp/0"
	peer, err := NewPeer(conf)
	require.NoError(t, err)

	pubKey, err := peer.PublicKey()
	require.NoError(t, err)

	pubKeyBytes, err := pubKey.Raw()
	require.NoError(t, err)

	conf.PersistentPeers = []*PeerInfo{{
		Address:   fmt.Sprintf("%v", peer.MultiAddresses()),
		PublicKey: pubKeyBytes,
	}}

	ctx := context.Background()
	txForwarder, err := forwarder.New(peer, 2*time.Second, func(tx *txsystem.Transaction) {})
	require.NoError(t, err)
	outProtocols := []OutProtocol{
		txForwarder,
	}
	NewLibP2PNetwork(ctx, peer, nil, outProtocols)
}*/
