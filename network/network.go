package network

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"slices"
	"time"

	"github.com/alphabill-org/alphabill/logger"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

type (
	MsgQueue interface {
		// Len - returns message queue len
		Len() int
		// PopFront - remove and return first message from queue (queue len is reduced by 1)
		PopFront() any
	}

	sendProtocolDescription struct {
		protocolID string
		msgType    any           // value of the message type of the protocol
		timeout    time.Duration // timeout per receiver
	}

	receiveProtocolDescription struct {
		protocolID string
		// constructor which returns pointer to a data struct into which
		// received message can be stored
		typeFn  func() any
		handler libp2pNetwork.StreamHandler
	}

	sendProtocolData struct {
		protocolID string
		timeout    time.Duration // per receiver timeout, ie when sending batch this is for each msg!
	}

	Observability interface {
		Tracer(name string, options ...trace.TracerOption) trace.Tracer
		Meter(name string, opts ...metric.MeterOption) metric.Meter
		Logger() *slog.Logger
	}
)

/*
LibP2PNetwork implements "alphabill network" using libp2p.

Zero value is not useable, use one of the constructors to create network!
*/
type LibP2PNetwork struct {
	self          *Peer
	sendProtocols map[reflect.Type]*sendProtocolData
	receivedMsgs  chan any // messages from LibP2PNetwork sent to this peer
	tracer        trace.Tracer
	log           *slog.Logger
}

/*
newLibP2PNetwork creates a new libp2p network without protocols (protocols need to be
registered separately to make the network actually useful).

In case of slow consumer up to "capacity" messages are buffered, after that messages will be dropped.

Logger (log) is assumed to already have node_id attribute added, won't be added by NW component!
*/
func newLibP2PNetwork(self *Peer, capacity uint, obs Observability) (*LibP2PNetwork, error) {
	if self == nil {
		return nil, errors.New("peer is nil")
	}

	n := &LibP2PNetwork{
		self:          self,
		sendProtocols: make(map[reflect.Type]*sendProtocolData),
		receivedMsgs:  make(chan any, capacity),
		tracer:        obs.Tracer("LibP2PNetwork"),
		log:           obs.Logger(),
	}
	return n, nil
}

func (n *LibP2PNetwork) ReceivedChannel() <-chan any {
	return n.receivedMsgs
}

// Send - send a single message to one or more peers asynchronously
func (n *LibP2PNetwork) Send(ctx context.Context, msg any, receivers ...peer.ID) error {
	if len(receivers) == 0 {
		return nil // no one to send message in single-node partition
		// it seems that current codebase depends on this behavior (returning nil instead of error)
		//return errors.New("at least one receiver ID must be provided")
	}

	p, f := n.sendProtocols[reflect.TypeOf(msg)]
	if !f {
		return fmt.Errorf("no protocol registered for messages of type %T", msg)
	}
	if err := n.sendAsync(ctx, p, msg, receivers); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}
	return nil
}

// SendMsgs - synchronously send a collection of the same type messages to peer (method can block)
// Returns successfully when all bytes have been written to the output buffer
// If during writing, the other side closes or resets the stream, an error will be returned
// However, this does not mean application level synchronization; messages can still be lost without the sender knowing
func (n *LibP2PNetwork) SendMsgs(ctx context.Context, messages MsgQueue, receiver peer.ID) (resErr error) {
	ctx, span := n.tracer.Start(ctx, "network.SenMsgs", trace.WithAttributes(attribute.Stringer("receiver", receiver)))
	defer span.End()
	var stream libp2pNetwork.Stream
	var err error
	for messages.Len() > 0 {
		msg := messages.PopFront()
		// create a stream with first message
		if stream == nil {
			p, f := n.sendProtocols[reflect.TypeOf(msg)]
			if !f {
				return fmt.Errorf("no protocol registered for messages of type %T", msg)
			}
			stream, err = n.self.CreateStream(ctx, receiver, p.protocolID)
			if err != nil {
				return fmt.Errorf("opening p2p stream %w", err)
			}
		}
		var data []byte
		data, err = serializeMsg(msg)
		if err != nil {
			// if serialization fails, then still try to send the rest
			resErr = errors.Join(resErr, fmt.Errorf("serializing message: %w", err))
			continue
		}

		// write message
		if _, err = stream.Write(data); err != nil {
			// return error on stream write error; it is unlikely that the other side is still able to process messages
			return errors.Join(resErr, fmt.Errorf("stream write error: %w", err))
		}
	}
	return nil
}

func (n *LibP2PNetwork) sendAsync(ctx context.Context, protocol *sendProtocolData, msg any, receivers []peer.ID) error {
	ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.sendAsync")
	defer span.End()

	data, err := serializeMsg(msg)
	if err != nil {
		return fmt.Errorf("serializing message: %w", err)
	}

	for _, receiver := range receivers {
		// loop-back for self-messages as libp2p would otherwise error:
		// open stream error: failed to dial: dial to self attempted
		if receiver == n.self.ID() {
			if err = n.receivedMsg(n.self.ID(), protocol.protocolID, msg); err != nil {
				// todo: this must be improved loop-back should not fail
				n.log.WarnContext(ctx, "message loop-back failed", logger.Error(err))
			}
			continue
		}
		go func(host *Peer, receiverID peer.ID) {
			ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.sendAsync.func", trace.WithNewRoot(), trace.WithLinks(trace.LinkFromContext(ctx)), trace.WithAttributes(attribute.String("protocol", protocol.protocolID)))
			defer span.End()
			// network nodes
			sendCtx, cancel := context.WithTimeout(ctx, protocol.timeout)
			defer cancel()
			if err := sendMsg(sendCtx, host, protocol.protocolID, data, receiverID); err != nil {
				n.log.WarnContext(sendCtx, fmt.Sprintf("sending %s to %v", protocol.protocolID, receiverID), logger.Error(err))
			}
		}(n.self, receiver)
	}
	return nil
}

func sendMsg(ctx context.Context, host *Peer, protocolID string, data []byte, receiverID peer.ID) (err error) {
	s, err := host.CreateStream(ctx, receiverID, protocolID)
	if err != nil {
		return fmt.Errorf("open p2p stream: %w", err)
	}
	defer func() {
		if closeErr := s.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("closing p2p stream: %w", closeErr))
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		if err = s.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("error setting write deadline: %w", err)
		}
	}
	if _, err = s.Write(data); err != nil {
		return fmt.Errorf("writing data to p2p stream: %w", err)
	}
	// done close the stream
	return nil
}

/*
streamHandlerForProtocol returns libp2p stream handler for given protocolID.
The "ctor" is constructor which returns pointer to a data struct into which
incoming message can be stored.
*/
func (n *LibP2PNetwork) streamHandlerForProtocol(protocolID string, ctor func() any) libp2pNetwork.StreamHandler {
	return func(s libp2pNetwork.Stream) {
		success := false
		defer func() {
			if success {
				if err := s.Close(); err != nil {
					n.log.Warn(fmt.Sprintf("closing p2p stream %q", protocolID), logger.Error(err))
				}
			} else {
				// stop writing to us, we have stopped reading
				if err := s.Reset(); err != nil {
					n.log.Warn(fmt.Sprintf("reset p2p stream %q", protocolID), logger.Error(err))
				}
			}
		}()
		// set reader timeout - node should not wait here forever
		err := s.SetReadDeadline(time.Now().Add(1000 * time.Millisecond))
		if err != nil {
			n.log.Warn(fmt.Sprintf("failed to set read deadline for stream %q", protocolID))
			return
		}
		reader := bufio.NewReader(s)
		for {
			msg := ctor()
			if err = deserializeMsg(reader, msg); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				n.log.Warn(fmt.Sprintf("reading %q message", protocolID), logger.Error(err))
				return
			}
			if err = n.receivedMsg(s.Conn().RemotePeer(), protocolID, msg); err != nil {
				// log error, but also reset the stream to signal that node is not able to consume more messages
				n.log.Warn(fmt.Sprintf("failed to process message: %v", err))
				return
			}
		}
		success = true
	}
}

func (n *LibP2PNetwork) receivedMsg(from peer.ID, protocolID string, msg any) error {
	select {
	case n.receivedMsgs <- msg:
	default:
		return fmt.Errorf("dropping %s message from %s because of slow consumer", protocolID, from)
	}
	return nil
}

func (n *LibP2PNetwork) registerReceiveProtocols(protocols []receiveProtocolDescription) error {
	if len(protocols) == 0 {
		return errors.New("at least one protocol description must be given")
	}

	for _, p := range protocols {
		if err := n.registerReceiveProtocol(p); err != nil {
			return fmt.Errorf("registering protocol %q: %w", p.protocolID, err)
		}
	}

	return nil
}

func (n *LibP2PNetwork) registerReceiveProtocol(protoc receiveProtocolDescription) error {
	if protoc.protocolID == "" {
		return errors.New("protocol ID must be assigned")
	}
	if slices.Contains(n.self.host.Mux().Protocols(), protocol.ID(protoc.protocolID)) {
		return fmt.Errorf("protocol %q is already registered", protoc.protocolID)
	}

	if protoc.handler != nil {
		n.self.RegisterProtocolHandler(protoc.protocolID, protoc.handler)
		return nil
	}

	if protoc.typeFn == nil {
		return errors.New("data struct constructor or handler must be assigned")
	}
	msg := protoc.typeFn()
	if msg == nil {
		return errors.New("data struct constructor returns nil")
	}
	switch typ := reflect.TypeOf(msg); typ.Kind() {
	case reflect.Pointer:
		if typ.Elem().Kind() != reflect.Struct {
			return fmt.Errorf("data struct constructor must return pointer to struct but returns %s", typ)
		}
		if reflect.ValueOf(msg).IsNil() {
			return fmt.Errorf("data struct constructor returns uninitialized pointer")
		}
	default:
		return fmt.Errorf("data struct constructor must return pointer to struct but returns %s", typ)
	}

	n.self.RegisterProtocolHandler(protoc.protocolID, n.streamHandlerForProtocol(protoc.protocolID, protoc.typeFn))
	return nil
}

/*
registerSendProtocols allows to register multiple send protocols with single call.
It calls "registerSendProtocol" for each element in the "protocols" parameter.
*/
func (n *LibP2PNetwork) registerSendProtocols(protocols []sendProtocolDescription) error {
	if len(protocols) == 0 {
		return errors.New("at least one protocol description must be given")
	}

	for _, pd := range protocols {
		if err := n.registerSendProtocol(pd); err != nil {
			return fmt.Errorf("registering protocol %q: %w", pd.protocolID, err)
		}
	}
	return nil
}

func (n *LibP2PNetwork) registerSendProtocol(protocol sendProtocolDescription) error {
	if protocol.protocolID == "" {
		return errors.New("protocol ID must be assigned")
	}

	if protocol.timeout < 0 {
		return fmt.Errorf("negative duration is not allowed for timeout, got %s for %s", protocol.timeout, protocol.protocolID)
	}

	typ := reflect.TypeOf(protocol.msgType)
	if typ == nil {
		return errors.New("message data type must be assigned")
	}
	if typ.Kind() != reflect.Struct {
		return fmt.Errorf("message data type must be struct, got %T", protocol.msgType)
	}

	if spd, ok := n.sendProtocols[typ]; ok {
		return fmt.Errorf("data type %s has been already registered for protocol %s", typ, spd.protocolID)
	}

	spx := &sendProtocolData{protocolID: protocol.protocolID, timeout: protocol.timeout}
	n.sendProtocols[typ] = spx
	n.sendProtocols[reflect.PointerTo(typ)] = spx
	return nil
}
