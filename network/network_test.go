package network

import (
	"context"
	"fmt"
	"testing"
	"time"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/stretchr/testify/require"
)

type testStrMsg struct {
	_    struct{} `cbor:",toarray"`
	Info string
}

type testMsgContainer struct {
	msgs []*testStrMsg
}

func (t *testMsgContainer) PushBack(msg *testStrMsg) {
	t.msgs = append(t.msgs, msg)
}

func (t *testMsgContainer) PopFront() any {
	if len(t.msgs) == 0 {
		panic("pop on empty container")
	}
	var msg *testStrMsg
	msg, t.msgs = t.msgs[0], t.msgs[1:]
	return msg
}

func (t *testMsgContainer) Len() int {
	return len(t.msgs)
}

func TestNewRootNodeLibP2PNetwork_Ok(t *testing.T) {
	peer := createPeer(t)
	defer func() { require.NoError(t, peer.Close()) }()
	net, err := NewLibP2PRootChainNetwork(peer, 1000, time.Second, observability.Default(t))
	require.NoError(t, err)
	require.NotNil(t, net)
	require.Equal(t, cap(net.ReceivedChannel()), 1000)
	require.Equal(t, 2, len(net.sendProtocols))
}

func Test_newLibP2PNetwork(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		net, err := newLibP2PNetwork(peer, 10, observability.NOPObservability())
		require.NoError(t, err)
		require.NotNil(t, net.self)
		require.Equal(t, cap(net.ReceivedChannel()), 10)
		require.Equal(t, 0, len(net.sendProtocols))
	})

	t.Run("peer is nil", func(t *testing.T) {
		net, err := newLibP2PNetwork(nil, 10, observability.NOPObservability())
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
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		net, err := newLibP2PNetwork(peer, 1, observability.Default(t))
		require.NoError(t, err)

		err = net.Send(context.Background(), testMsg{})
		require.EqualError(t, err, `at least one receiver ID must be provided`)
	})

	t.Run("no receiver provided is not error", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		net, err := newLibP2PNetwork(peer, 1, observability.Default(t))
		require.NoError(t, err)
		// seems wrong but current codebase depends on this behavior?
		require.NoError(t, net.Send(context.Background(), testMsg{}))
	})

	t.Run("unknown message type", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		net, err := newLibP2PNetwork(peer, 1, observability.Default(t))
		require.NoError(t, err)

		var msg = struct{ foo int }{}
		err = net.Send(context.Background(), msg, peer.ID())
		require.EqualError(t, err, `no protocol registered for messages of type struct { foo int }`)

		err = net.Send(context.Background(), "foobar", peer.ID())
		require.EqualError(t, err, `no protocol registered for messages of type string`)
	})

	t.Run("failure to serialize message", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		net, err := newLibP2PNetwork(peer, 1, observability.Default(t))
		require.NoError(t, err)
		require.NoError(t, net.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: noCBOR{}, timeout: 100 * time.Millisecond}))

		msg := noCBOR{Name: "msg", Value: 1024}
		err = net.Send(context.Background(), msg, peer.ID())
		require.EqualError(t, err, `sending message: serializing message: marshaling network.noCBOR as CBOR: no CBOR for this type`)
	})

	t.Run("success, message to self", func(t *testing.T) {
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, observability.Default(t))
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
		obs := observability.Default(t)
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
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
		obs := observability.Default(t)
		// create peer for sender and two receivers
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)

		peer3 := createPeer(t)
		defer func() { require.NoError(t, peer3.Close()) }()
		nw3, err := newLibP2PNetwork(peer3, 1, obs)
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

func Test_LibP2PNetwork_SendMsgs(t *testing.T) {
	t.Run("success, messages to other peer", func(t *testing.T) {
		obs := observability.Default(t)
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 2, obs)
		require.NoError(t, err)
		// need to init peerstore manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testStrMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testStrMsg{} }}))
		msgQueue := &testMsgContainer{}
		msgQueue.PushBack(&testStrMsg{Info: "test message1"})
		msgQueue.PushBack(&testStrMsg{Info: "test message2"})

		require.NoError(t, nw1.SendMsgs(context.Background(), msgQueue, peer2.ID()))
		// wait for messages received
		require.Eventually(t, func() bool { return len(nw2.receivedMsgs) == 2 }, test.WaitDuration, test.WaitTick)
		// message one
		rm := <-nw2.ReceivedChannel()
		require.EqualValues(t, "test message1", rm.(*testStrMsg).Info)
		rm = <-nw2.ReceivedChannel()
		require.EqualValues(t, "test message2", rm.(*testStrMsg).Info)
	})
	t.Run("fails, receiver has room for only one message", func(t *testing.T) {
		obs := observability.Default(t)
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		// need to init peerstore manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testStrMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testStrMsg{} }}))
		msgQueue := &testMsgContainer{}
		for i := 1; i <= 4; i++ {
			msgQueue.PushBack(&testStrMsg{Info: fmt.Sprintf("make a test message that is a bit longer to simulate real messages: test message %v", i)})
		}
		// NB! All messages are sent successfully? - actually, no 3 messages get dropped.
		// Since all messages were successfully added to the out buffer and also received,
		// but later dropped - there is no error here.
		require.NoError(t, nw1.SendMsgs(context.Background(), msgQueue, peer2.ID()))
		// wait for messages received
		require.Eventually(t, func() bool { return len(nw2.receivedMsgs) == 1 }, test.WaitDuration, test.WaitTick)
		// message one
		rm := <-nw2.ReceivedChannel()
		require.EqualValues(t, "make a test message that is a bit longer to simulate real messages: test message 1", rm.(*testStrMsg).Info)
	})
	t.Run("fails, stream reset by receiver while still sending", func(t *testing.T) {
		obs := observability.Default(t)
		peer1 := createPeer(t)
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		// need to init peerstore manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testStrMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testStrMsg{} }}))
		msgQueue := &testMsgContainer{}
		for i := 1; i <= 10000; i++ {
			msgQueue.PushBack(&testStrMsg{Info: fmt.Sprintf("make a test message that is a bit longer to simulate real messages: test message %v", i)})
		}
		require.ErrorContains(t, nw1.SendMsgs(context.Background(), msgQueue, peer2.ID()), "stream write error: stream reset")
	})
	t.Run("unknown protocol type", func(t *testing.T) {
		obs := observability.Default(t)
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		// need to init peerstore manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		type fooMsg struct{}
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: fooMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testStrMsg{} }}))
		msgQueue := &testMsgContainer{}
		for i := 1; i <= 4; i++ {
			msgQueue.PushBack(&testStrMsg{Info: fmt.Sprintf("make a test message that is a bit longer to simulate real messages: test message %v", i)})
		}
		require.EqualError(t, nw1.SendMsgs(context.Background(), msgQueue, peer2.ID()), "no protocol registered for messages of type *network.testStrMsg")
	})
	t.Run("not able to dial", func(t *testing.T) {
		obs := observability.Default(t)
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testStrMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testStrMsg{} }}))
		msgQueue := &testMsgContainer{}
		for i := 1; i <= 4; i++ {
			msgQueue.PushBack(&testStrMsg{Info: fmt.Sprintf("make a test message that is a bit longer to simulate real messages: test message %v", i)})
		}
		require.EqualError(t, nw1.SendMsgs(context.Background(), msgQueue, peer2.ID()), "opening p2p stream failed to find any peer in table")
	})
}

func Test_LibP2PNetwork_sendMsg(t *testing.T) {
	t.Run("unknown protocol", func(t *testing.T) {
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, observability.Default(t))
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		require.NoError(t, err)

		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		// networks have no protocols registered so sending data must fail
		msg := []byte{3, 2, 1}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err = sendMsg(ctx, nw1.self, "test/p", msg, peer2.ID())
		require.EqualError(t, err, `open p2p stream: failed to negotiate protocol: protocols not supported: [test/p]`)
	})

	type testMsg struct {
		_     struct{} `cbor:",toarray"`
		Name  string
		Value int
	}

	t.Run("unknown peer", func(t *testing.T) {
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, observability.Default(t))
		require.NoError(t, err)
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		require.NoError(t, err)

		// do NOT init peerstores so peers can't dial each other
		//peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		//peer2.Network().Peerstore().AddAddrs(peer1.ID(), peer1.MultiAddresses(), peerstore.PermanentAddrTTL)

		msg := []byte{3, 2, 1}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err = sendMsg(ctx, nw1.self, "test/p", msg, peer2.ID())
		require.EqualError(t, err, "open p2p stream: failed to find any peer in table")
	})

	t.Run("connection refused", func(t *testing.T) {
		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, observability.Default(t))
		require.NoError(t, err)
		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		require.NoError(t, err)

		// need to init peerstores manually, otherwise peers can't dial each other...
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)
		// ...but close peer2 network connection
		require.NoError(t, peer2.Close())

		msg := []byte{3, 2, 1}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		err = sendMsg(ctx, nw1.self, "test/p", msg, peer2.ID())
		require.ErrorContains(t, err, fmt.Sprintf("open p2p stream: failed to dial: failed to dial %s: all dials failed", peer2.ID()))
		require.ErrorContains(t, err, `connection refused`)
	})

	t.Run("context cancelled", func(t *testing.T) {
		obs := observability.Default(t)

		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		msg := []byte{3, 2, 1}
		ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
		defer cancel()
		err = sendMsg(ctx, nw1.self, "test/p", msg, peer2.ID())
		require.EqualError(t, err, `open p2p stream: failed to dial: context canceled`)
	})

	t.Run("success", func(t *testing.T) {
		obs := observability.Default(t)

		peer1 := createPeer(t)
		defer func() { require.NoError(t, peer1.Close()) }()
		nw1, err := newLibP2PNetwork(peer1, 1, obs)
		require.NoError(t, err)

		peer2 := createPeer(t)
		defer func() { require.NoError(t, peer2.Close()) }()
		nw2, err := newLibP2PNetwork(peer2, 1, obs)
		require.NoError(t, err)
		// need to init peerstores manually, otherwise peers can't dial each other
		peer1.Network().Peerstore().AddAddrs(peer2.ID(), peer2.MultiAddresses(), peerstore.PermanentAddrTTL)

		require.NoError(t, nw1.registerSendProtocol(sendProtocolDescription{protocolID: "test/p", msgType: testMsg{}, timeout: 100 * time.Millisecond}))
		require.NoError(t, nw2.registerReceiveProtocol(receiveProtocolDescription{protocolID: "test/p", typeFn: func() any { return &testMsg{} }}))

		msg := &testMsg{Name: "oh my!", Value: 555}
		data, err := serializeMsg(msg)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		require.NoError(t, sendMsg(ctx, nw1.self, "test/p", data, peer2.ID()))

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
	defer func() { require.NoError(t, peer.Close()) }()
	obs := observability.NOPObservability()

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)
		require.NoError(t, nw.registerSendProtocols([]sendProtocolDescription{validSendProtocolDescription()}), `expected validSendProtocolDescription to return valid data`)
	})

	t.Run("empty input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		require.EqualError(t, nw.registerSendProtocols(nil), `at least one protocol description must be given`)
		require.EqualError(t, nw.registerSendProtocols([]sendProtocolDescription{}), `at least one protocol description must be given`)
	})

	t.Run("invalid input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
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
	defer func() { require.NoError(t, peer.Close()) }()
	obs := observability.NOPObservability()

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)
		require.NoError(t, nw.registerSendProtocol(validSendProtocolDescription()), `expected validSendProtocolDescription to return valid data`)
	})

	t.Run("protocol ID unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.protocolID = ""
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `protocol ID must be assigned`)
	})

	t.Run("message data type unassigned", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = nil
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be assigned`)
	})

	t.Run("message data type is not struct", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = "foobar"
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be struct, got string`)
	})

	t.Run("message data type is pointer to struct", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		data.msgType = &testMsg{}
		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `message data type must be struct, got *network.testMsg`)
	})

	t.Run("attempt to register same data type twice", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validSendProtocolDescription()
		require.NoError(t, err, nw.registerSendProtocol(data), "expected first attempt to succeed")

		err = nw.registerSendProtocol(data)
		require.EqualError(t, err, `data type network.testMsg has been already registered for protocol foo/bar`)
	})

	t.Run("negative timeout", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
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
	defer func() { require.NoError(t, peer.Close()) }()
	obs := observability.NOPObservability()

	t.Run("valid", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)
		require.NoError(t, nw.registerReceiveProtocols([]receiveProtocolDescription{validReceiveProtocolDescription()}), `expected validReceiveProtocolDescription to return valid data`)
	})

	t.Run("empty input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		require.EqualError(t, nw.registerReceiveProtocols(nil), `at least one protocol description must be given`)
		require.EqualError(t, nw.registerReceiveProtocols([]receiveProtocolDescription{}), `at least one protocol description must be given`)
	})

	t.Run("invalid input", func(t *testing.T) {
		nw, err := newLibP2PNetwork(peer, 1, obs)
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
	obs := observability.NOPObservability()

	t.Run("attempt to register same type multiple times", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		require.NoError(t, err, nw.registerReceiveProtocol(data), "expected first attempt to succeed")

		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `protocol "foo/bar" is already registered`)
	})

	t.Run("protocol ID unassigned", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		data.protocolID = ""
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `protocol ID must be assigned`)
	})

	t.Run("constructor func unassigned", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		nw, err := newLibP2PNetwork(peer, 1, obs)
		require.NoError(t, err)

		data := validReceiveProtocolDescription()
		data.typeFn = nil
		err = nw.registerReceiveProtocol(data)
		require.EqualError(t, err, `data struct constructor or handler must be assigned`)
	})

	t.Run("constructor returns invalid type", func(t *testing.T) {
		peer := createPeer(t)
		defer func() { require.NoError(t, peer.Close()) }()
		nw, err := newLibP2PNetwork(peer, 1, obs)
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
