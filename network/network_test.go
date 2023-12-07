package network

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/stretchr/testify/require"
)

func TestNewValidatorLibP2PNetwork_Ok(t *testing.T) {
	obs := observability.NOPMetrics()
	net, err := NewLibP2PValidatorNetwork(createPeer(t), DefaultValidatorNetworkOptions, obs, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, net)
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	// we register protocol for each message for both value and pointer type thus
	// there must be twice the amount of items in the sendProtocols map than the
	// actual supported message types is
	require.Equal(t, 10, len(net.sendProtocols))
}

func TestNewRootNodeLibP2PNetwork_Ok(t *testing.T) {
	net, err := NewLibP2PRootChainNetwork(createPeer(t), 1000, time.Second, logger.New(t))
	require.NoError(t, err)
	require.NotNil(t, net)
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	require.Equal(t, 2, len(net.sendProtocols))
}

func Test_newLibP2PNetwork(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		net, err := newLibP2PNetwork(createPeer(t), 10, logger.NOP())
		require.NoError(t, err)
		require.NotNil(t, net.self)
		require.Equal(t, cap(net.ReceivedChannel()), 10)
		require.Equal(t, 0, len(net.sendProtocols))
	})

	t.Run("peer is nil", func(t *testing.T) {
		net, err := newLibP2PNetwork(nil, 10, logger.NOP())
		require.EqualError(t, err, `peer is nil`)
		require.Nil(t, net)
	})
}

func Test_LibP2PNetwork_Send(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	t.Run("no receiver provided", func(t *testing.T) {
		t.Skip("seems that codebase currently depends on not-error behavior!")
		net, err := newLibP2PNetwork(createPeer(t), 1, logger.New(t))
		require.NoError(t, err)

		err = net.Send(context.Background(), testMsg{})
		require.EqualError(t, err, `at least one receiver ID must be provided`)
	})
	t.Run("no receiver provided is not error", func(t *testing.T) {
		net, err := newLibP2PNetwork(createPeer(t), 1, logger.New(t))
		require.NoError(t, err)
		// seems wrong but current codebase depends on this behavior?
		require.NoError(t, net.Send(context.Background(), testMsg{}))
	})

	t.Run("unknown message type", func(t *testing.T) {
		peer := createPeer(t)
		net, err := newLibP2PNetwork(peer, 1, logger.New(t))
		require.NoError(t, err)

		var msg = struct{ foo int }{}
		err = net.Send(context.Background(), msg, peer.ID())
		require.EqualError(t, err, `no protocol registered for messages of type struct { foo int }`)

		err = net.Send(context.Background(), "foobar", peer.ID())
		require.EqualError(t, err, `no protocol registered for messages of type string`)
	})

	t.Run("failure to serialize message", func(t *testing.T) {
		peer := createPeer(t)
		net, err := newLibP2PNetwork(peer, 1, logger.New(t))
		require.NoError(t, err)
		require.NoError(t, net.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: noCBOR{}, timeout: 100 * time.Millisecond}))

		msg := noCBOR{Name: "msg", Value: 1024}
		err = net.Send(context.Background(), msg, peer.ID())
		require.EqualError(t, err, `sending message: serializing message: marshaling network.noCBOR as CBOR: no CBOR for this type`)
	})

	t.Run("success, message to self", func(t *testing.T) {
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, logger.New(t))
		require.NoError(t, err)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		// sending message to self uses "loopback" so shouldn't need to register receive protocol
		//require.NoError(t, nw1.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		msg := testMsg{Name: "test message", Value: 127}
		require.NoError(t, nw1.Send(context.Background(), msg, peer1.ID()))

		select {
		case rm := <-nw1.ReceivedChannel():
			require.Equal(t, msg, rm)
		case <-time.After(time.Second):
			t.Error("haven't got message before timeout")
		}
	})

	t.Run("success, message to other peer", func(t *testing.T) {
		log := logger.New(t)
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, log)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, log)
		require.NoError(t, err)
		// need to init peerstore manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		msg := &testMsg{Name: "test message", Value: 127}
		require.NoError(t, nw1.Send(context.Background(), msg, peer2.ID()))

		select {
		case rm := <-nw2.ReceivedChannel():
			require.Equal(t, msg, rm)
		case <-time.After(time.Second):
			t.Error("haven't got message before timeout")
		}
	})

	t.Run("success, message to two peers", func(t *testing.T) {
		log := logger.New(t)
		// create peer for sender and two receivers
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, log)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, log)
		require.NoError(t, err)

		peer3 := createPeer(t)
		nw3, err := newLibP2PNetwork(peer3, 1, log)
		require.NoError(t, err)

		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		peer1.Network().Peerstore().AddAddrs(peer3.ID(), peer3.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		rpd := receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}
		require.NoError(t, nw2.registerReceiveProtocol(rpd))
		require.NoError(t, nw3.registerReceiveProtocol(rpd))

		msg := &testMsg{Name: "test message", Value: 127}
		require.NoError(t, nw1.Send(context.Background(), msg, peer2.ID(), peer3.ID()))

		select {
		case rm := <-nw2.ReceivedChannel():
			require.Equal(t, msg, rm)
		case <-time.After(time.Second):
			t.Error("nw2 haven't got message before timeout")
		}

		select {
		case rm := <-nw3.ReceivedChannel():
			require.Equal(t, msg, rm)
		case <-time.After(time.Second):
			t.Error("nw3 haven't got message before timeout")
		}
	})
}

func Test_LibP2PNetwork_sendMsg(t *testing.T) {
	t.Run("unknown protocol", func(t *testing.T) {
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, logger.New(t))
		require.NoError(t, err)

		peer2 := createPeer(t)
		require.NoError(t, err)

		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		// networks have no protocols registered so sending data must fail
		msg := []byte{3, 2, 1}
		err = nw1.sendMsg(context.Background(), msg, "test/p", 100*time.Millisecond, peer2.ID())
		require.EqualError(t, err, `open p2p stream: failed to negotiate protocol: protocols not supported: [test/p]`)
	})

	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	t.Run("unknown peer", func(t *testing.T) {
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, logger.New(t))
		require.NoError(t, err)
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))

		peer2 := createPeer(t)
		require.NoError(t, err)

		// do NOT init peerstores so peers can't dial each other
		//peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		//peer2.Network().Peerstore().AddAddrs(peer1.ID(), peer1.MultiAddresses(), peerstore.PermanentAddrTTL)

		msg := []byte{3, 2, 1}
		err = nw1.sendMsg(context.Background(), msg, "test/p", 100*time.Millisecond, peer2.ID())
		require.EqualError(t, err, fmt.Sprintf("open p2p stream: failed to dial: failed to dial %s: no addresses", peer2.ID()))
	})

	t.Run("connection refused", func(t *testing.T) {
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, logger.New(t))
		require.NoError(t, err)
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))

		peer2 := createPeer(t)
		require.NoError(t, err)

		// need to init peerstores manually, otherwise peers can't dial each other...
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		// ...but close peer2 network connection
		peer2.Close()

		msg := []byte{3, 2, 1}
		err = nw1.sendMsg(context.Background(), msg, "test/p", 100*time.Millisecond, peer2.ID())
		require.ErrorContains(t, err, fmt.Sprintf("open p2p stream: failed to dial: failed to dial %s: all dials failed", peer2.ID()))
		require.ErrorContains(t, err, `connection refused`)
	})

	t.Run("context cancelled", func(t *testing.T) {
		log := logger.New(t)

		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, log)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, log)
		require.NoError(t, err)
		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msg := []byte{3, 2, 1}
		err = nw1.sendMsg(ctx, msg, "test/p", 100*time.Millisecond, peer2.ID())
		require.EqualError(t, err, `open p2p stream: failed to dial: context canceled`)
	})

	t.Run("success", func(t *testing.T) {
		log := logger.New(t)

		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, log)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, log)
		require.NoError(t, err)
		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		msg := &testMsg{Name: "oh my!", Value: 555}
		data, err := serializeMsg(msg)
		require.NoError(t, err)
		require.NoError(t, nw1.sendMsg(context.Background(), data, "test/p", 100*time.Millisecond, peer2.ID()))

		select {
		case rm := <-nw2.ReceivedChannel():
			require.Equal(t, msg, rm)
		case <-time.After(time.Second):
			t.Error("nw2 haven't got message before timeout")
		}
	})
}

func Test_LibP2PNetwork_registerSendProtocols(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	validSendProtocolDescription := func() sendProtocolDescription {
		return sendProtocolDescription{
			protocolID: "foo/bar",
			msgType:    testMsg{},
			timeout:    time.Second,
		}
	}

	// we can reuse the peer for every subtest
	peer := createPeer(t)

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)
		require.NoError(t, nw.registerSendProtocols([]sendProtocolDescription{validSendProtocolDescription()}), `expected validSendProtocolDescription to return valid data`)
	})

	t.Run("empty input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		require.EqualError(t, nw.registerSendProtocols(nil), `at least one protocol description must be given`)
		require.EqualError(t, nw.registerSendProtocols([]sendProtocolDescription{}), `at least one protocol description must be given`)
	})

	t.Run("invalid input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.protocolID = ""
		err = nw.registerSendProtocols([]sendProtocolDescription{data})
		require.EqualError(t, err, `registering protocol "": protocol ID must be assigned`)
	})
}

func Test_LibP2PNetwork_registerSendProtocol(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	validSendProtocolDescription := func() sendProtocolDescription {
		return sendProtocolDescription{
			protocolID: "foo/bar",
			msgType:    testMsg{},
			timeout:    time.Second,
		}
	}

	// we can reuse the peer for every subtest
	peer := createPeer(t)

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)
		require.NoError(t, nw.registerSendProtocol(validSendProtocolDescription()), `expected validSendProtocolDescription to return valid data`)
	})

	t.Run("protocol ID unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.protocolID = ""
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `protocol ID must be assigned`)
	})

	t.Run("message data type unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = nil
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be assigned`)
	})

	t.Run("message data type is not struct", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = "foobar"
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be struct, got string`)
	})

	t.Run("message data type is pointer to struct", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = &testMsg{}
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be struct, got *network.testMsg`)
	})

	t.Run("attempt to register same data type twice", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		require.NoError(t, err, nw.registerSendProtocol(data), "expected first attempt to succeed")

		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `data type network.testMsg has been already registered for protocol foo/bar`)
	})

	t.Run("negative timeout", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.timeout = -1 * time.Millisecond
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `negative duration is not allowed for timeout, got -1ms for foo/bar`)
	})
}

func Test_LibP2PNetwork_registerReceiveProtocols(t *testing.T) {

	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	validReceiveProtocolDescription := func() receiveProtocolDescription {
		return receiveProtocolDescription{
			protocolID: "foo/bar",
			typeFn:     func() any { return &testMsg{} },
		}
	}

	// we can reuse the peer for every subtest
	peer := createPeer(t)

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)
		require.NoError(t, nw.registerReceiveProtocols([]receiveProtocolDescription{validReceiveProtocolDescription()}), `expected validReceiveProtocolDescription to return valid data`)
	})

	t.Run("empty input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		require.EqualError(t, nw.registerReceiveProtocols(nil), `at least one protocol description must be given`)
		require.EqualError(t, nw.registerReceiveProtocols([]receiveProtocolDescription{}), `at least one protocol description must be given`)
	})

	t.Run("invalid input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, nil)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		data.protocolID = ""
		err = nw.registerReceiveProtocols([]receiveProtocolDescription{data})
		require.EqualError(t, err, `registering protocol "": protocol ID must be assigned`)
	})
}

func Test_LibP2PNetwork_registerReceiveProtocol(t *testing.T) {
	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	validReceiveProtocolDescription := func() receiveProtocolDescription {
		return receiveProtocolDescription{
			protocolID: "foo/bar",
			typeFn:     func() any { return &testMsg{} },
		}
	}

	t.Run("attempt to register same type multiple times", func(t *testing.T) {
		nw, err := newLibP2PNetwork(createPeer(t), 1, nil)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		require.NoError(t, err, nw.registerReceiveProtocol(data), "expected first attempt to succeed")

		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `protocol "foo/bar" is already registered`)
	})

	t.Run("protocol ID unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(createPeer(t), 1, nil)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		data.protocolID = ""
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `protocol ID must be assigned`)
	})

	t.Run("constructor func unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(createPeer(t), 1, nil)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		data.typeFn = nil
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor or handler must be assigned`)
	})

	t.Run("constructor returns invalid type", func(t *testing.T) {
		nw, err := newLibP2PNetwork(createPeer(t), 1, nil)
		require.NoError(t, err)
		data := validReceiveProtocolDescription()

		// untyped nil
		data.typeFn = func() any { return nil }
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor returns nil`)

		// typed nil (struct type)
		data.typeFn = func() any { var v *testMsg; return v }
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor returns uninitialized pointer`)

		// not a struct type
		data.typeFn = func() any { return "" }
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor must return pointer to struct but returns string`)

		// pointer to non-struct type
		data.typeFn = func() any { var v *int; return v }
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor must return pointer to struct but returns *int`)

		// struct, not pointer to struct
		data.typeFn = func() any { return testMsg{} }
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor must return pointer to struct but returns network.testMsg`)
	})
}
