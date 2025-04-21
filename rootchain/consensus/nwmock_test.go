package consensus

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

const AnyPeer peer.ID = "*"

/*
newMockNetwork creates mocked AB network for test "t".
*/
func newMockNetwork(t *testing.T) *mockNetwork {
	mnw := &mockNetwork{
		t:      t,
		cons:   make(map[peer.ID]chan any),
		sendTO: 70 * time.Millisecond,
	}
	mnw.firewall.Store(fwFunc(nil))
	return mnw
}

type fwFunc func(from, to peer.ID, msg any) bool

type mockNetwork struct {
	t      *testing.T
	cons   map[peer.ID]chan any
	sendTO time.Duration // timeout for send operation

	// firewall stores func (of type fwFunc) which returns "true" when the message
	// should be blocked and "false" when msg should pass through FW
	firewall atomic.Value

	m    sync.Mutex
	errs []error
}

/*
Connect creates mocked RootNet for given peer, the peer is "connected" to the
same network with other peers in the mockNetwork.
NB! not concurrency safe, register peers before concurrent action!
*/
func (mnw *mockNetwork) Connect(node peer.ID) *mockNwConnection {
	mnw.t.Helper()

	if _, exists := mnw.cons[node]; exists {
		mnw.t.Fatalf("mockNetwork.Connect should be called only once, peer %s is already connected", node)
	}

	con := &mockNwConnection{nw: mnw, id: node, rcv: make(chan any)}
	mnw.cons[node] = con.rcv
	return con
}

/*
SetFirewall replaces current firewall func.

When the FW func returns "true" the message will be blocked, when "false" the msg
is passed on. Use "nil" to disable FW (ie all messages will be passed on without
filter). Message blocked by FW just disappears, no error is returned to the sender.

Firewall can be used to eavesdrop on traffic but keep in mind that the firewall sees
the message before peer receives it (and peer might even never see it as it might be
overloaded and send timeout kicks in before peer accepts the message)!
*/
func (mnw *mockNetwork) SetFirewall(fw fwFunc) {
	mnw.firewall.Store(fw)
}

func (mnw *mockNetwork) logError(err error) {
	mnw.m.Lock()
	defer mnw.m.Unlock()
	mnw.errs = append(mnw.errs, err)
}

/*
Send posts (synchronous) message "msg" to the peers listed as "receivers".

Messages sent with this method do not go through firewall (set with SetFirewall)!

Test (for which the network has been created) will fail when:
  - attempting to send to unknown peer;
  - peer doesn't consume the message within timeout.
*/
func (mnw *mockNetwork) Send(msg any, receivers ...peer.ID) {
	mnw.t.Helper()
	if len(receivers) == 0 {
		mnw.t.Error("calling Send with no receiver(s)")
	}

	for _, addr := range receivers {
		con, ok := mnw.cons[addr]
		if !ok {
			mnw.t.Errorf("unknown receiver %s for message %#v", addr, msg)
			continue
		}

		select {
		case <-time.After(mnw.sendTO):
			mnw.t.Errorf("send operation to %s timed out for message %#v", addr, msg)
		case con <- msg:
		}
	}
}

func (mnw *mockNetwork) send(from, to peer.ID, msg any) error {
	con, ok := mnw.cons[to]
	if !ok {
		return fmt.Errorf("unknown receiver %s for message %#v sent by %s", to, msg, from)
	}

	go func() {
		if fw := mnw.firewall.Load().(fwFunc); fw != nil {
			// if firewall fn returns true, the message is blocked
			if fw(from, to, msg) {
				return
			}
		}
		select {
		case <-time.After(mnw.sendTO):
			mnw.logError(fmt.Errorf("send operation timed out %s -> %s : %#v", from, to, msg))
		case con <- msg:
		}
	}()

	return nil
}

type mockNwConnection struct {
	nw  *mockNetwork
	id  peer.ID
	rcv chan any
}

func (nc *mockNwConnection) Send(_ context.Context, msg any, receivers ...peer.ID) error {
	var errs []error
	for _, id := range receivers {
		if err := nc.nw.send(nc.id, id, msg); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (nc *mockNwConnection) ReceivedChannel() <-chan any {
	return nc.rcv
}

/*
ForwardMsgs returns a function which can be used as firewall in mockNetwork (ie used
as parameter in mockNetwork.SetFirewall call). It always returns false (so doesn't
block any message) but messages which match the sender/receiver filter and are of
the same "base type" as the "wire" chan element are sent to the "wire" chan.
Note that the send is synchronous ie the message is blocked until send to wire chan
succeeds.

If unsure whether the messages are values or pointers it's safer to err on the side
of values, ie prefer

	wire := make(chan MsgStructType, 1)

to

	wire := make(chan *MsgStructType, 1)

In order not to filter by sender and/or receiver AnyPeer can be used as filter
value of appropriate parameter.
*/
func ForwardMsgs[T any](from, to peer.ID, wire chan<- T) fwFunc {
	mt := reflect.TypeOf(new(T)).Elem()
	return func(parFrom, parTo peer.ID, msg any) bool {
		if (from == AnyPeer || from == parFrom) && (to == AnyPeer || to == parTo) {
			if v := convertTo(msg, mt); v != nil {
				wire <- v.(T)
			}
		}
		return false
	}
}

func convertTo(v any, typ reflect.Type) any {
	rtt, tcnt := rootType(typ, 0)
	rtv, vcnt := rootType(reflect.TypeOf(v), 0)
	if rtt != rtv {
		return nil
	}
	if tcnt == vcnt {
		return v
	}

	val := reflect.ValueOf(v)
	switch {
	case tcnt > vcnt:
		for ; tcnt > vcnt; tcnt-- {
			// likely to fail and panic... :(
			// and we rather panic than fail silently (ie no CanAddr) check
			// this is why it's safer to filter for value rather than pointer
			// types (see comment for ForwardMsgs func)
			val = val.Addr()
		}
	case tcnt < vcnt:
		for ; vcnt > tcnt; vcnt-- {
			val = val.Elem()
		}
	}
	return val.Interface()
}

/*
rootType returns the "base type" of "t" and number of indirections
the type has ("pointer depth").
*/
func rootType(t reflect.Type, cnt int) (reflect.Type, int) {
	if t.Kind() == reflect.Pointer {
		return rootType(t.Elem(), cnt+1)
	}
	return t, cnt
}

func Test_convertTo(t *testing.T) {
	t.Parallel()

	type MsgA struct {
		Value string
	}

	ptrA := func(a *MsgA) any { return &a }

	// cases where conversion should succeed
	var testCases = []struct {
		name  string // comment for the test case
		value any    // input value
		dest  any    // type we want to convert to
	}{
		{name: "values",
			value: MsgA{},
			dest:  MsgA{},
		},
		{name: "pointers",
			value: &MsgA{},
			dest:  &MsgA{},
		},
		{name: "pointer to value",
			value: &MsgA{},
			dest:  MsgA{},
		},
		{name: "new to pointer",
			value: new(MsgA), // == &MsgA{}
			dest:  &MsgA{},
		},
		{name: "pointer to pointer -> pointer",
			value: new(*MsgA),
			dest:  &MsgA{},
		},
		{name: "**pointer to value",
			value: ptrA(&MsgA{}),
			dest:  MsgA{},
		},
		{name: "**pointer to value",
			value: ptrA(&MsgA{}),
			dest:  &MsgA{},
		},
	}

	for _, tc := range testCases {
		dstTyp := reflect.TypeOf(tc.dest)
		cnv := convertTo(tc.value, dstTyp)
		if typ := reflect.TypeOf(cnv); typ != dstTyp {
			t.Errorf("expected %s got %s for %q", dstTyp, typ, tc.name)
		}
	}

	// bad case, attempting to get pointer to the value
	require.PanicsWithValue(t,
		`reflect.Value.Addr of unaddressable value`,
		func() { convertTo(MsgA{}, reflect.TypeOf(&MsgA{})) },
	)

	// "normal" failure cases

	type MsgB struct {
		Value int
	}
	require.Nil(t, convertTo(MsgA{}, reflect.TypeOf(MsgB{})), `cant convert A to B`)
	require.Nil(t, convertTo(&MsgA{}, reflect.TypeOf(MsgB{})), `cant convert *A to B`)
}
