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
	"sync"
	"time"

	"github.com/alphabill-org/alphabill/logger"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const LargeMessageTimeout = 1 * time.Second

type (
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

// Send - send single message to one or more peers
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
	if err := n.send(ctx, p, msg, receivers); err != nil {
		return fmt.Errorf("sending message: %w", err)
	}

	return nil
}

// SendMessages - send multiple messages at once to one recipient
func (n *LibP2PNetwork) SendMessages(ctx context.Context, messages []any, receiverID peer.ID) (err error) {
	ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.sendmessages")
	defer span.End()
	var stream libp2pNetwork.Stream
	for _, msg := range messages {
		p, f := n.sendProtocols[reflect.TypeOf(msg)]
		if !f {
			return fmt.Errorf("no protocol registered for messages of type %T", msg)
		}
		if stream == nil {
			stream, err = n.self.CreateStream(ctx, receiverID, p.protocolID)
			if err != nil {
				return fmt.Errorf("open p2p stream: %w", err)
			}
			defer func() {
				if err != nil {
					if resErr := stream.Reset(); resErr != nil {
						err = errors.Join(err, resErr)
					}
				} else {
					if closeErr := stream.Close(); closeErr != nil {
						err = errors.Join(err, closeErr)
					}
				}
			}()
		}
		deadline, _ := ctx.Deadline()
		err = stream.SetWriteDeadline(deadline)
		data, err := serializeMsg(msg)
		if err != nil {
			// skip this and still try to send others
			n.log.WarnContext(ctx, "serializing %q message", p.protocolID, logger.Error(err))
			continue
		}
		if _, err = stream.Write(data); err != nil {

		}
	}
	return nil
}

func (n *LibP2PNetwork) send(ctx context.Context, protocol *sendProtocolData, msg any, receivers []peer.ID) error {
	ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.send")
	defer span.End()

	data, err := serializeMsg(msg)
	if err != nil {
		return fmt.Errorf("serializing message: %w", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(receivers))
	for _, receiver := range receivers {
		// loop-back for self-messages as libp2p would otherwise error:
		// open stream error: failed to dial: dial to self attempted
		if receiver == n.self.ID() {
			n.receivedMsg(n.self.ID(), protocol.protocolID, msg)
			continue
		}
		wg.Add(1)
		go func(host *Peer, receiverID peer.ID) {
			defer wg.Done()
			ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.send.func", trace.WithNewRoot(), trace.WithLinks(trace.LinkFromContext(ctx)), trace.WithAttributes(attribute.String("protocol", protocol.protocolID)))
			defer span.End()
			// network nodes
			sendCtx, cancel := context.WithTimeout(ctx, protocol.timeout)
			defer cancel()
			s, err := host.CreateStream(sendCtx, receiverID, protocol.protocolID)
			if err != nil {
				errs <- fmt.Errorf("open p2p stream: %w", err)
				return
			}
			deadline, _ := sendCtx.Deadline()
			err = s.SetWriteDeadline(deadline)
			if _, err = s.Write(data); err != nil {
				// on error reset to make sure that the next stream is not affected by the same error
				// reset forces close of both ends of the stream
				if resetErr := s.Reset(); resetErr != nil {
					errs <- errors.Join(fmt.Errorf("writing data to p2p stream: %w", err), fmt.Errorf("stream reset: %w", resetErr))
					return
				}
				errs <- fmt.Errorf("writing data to p2p stream: %w", err)
				return
			}
			if err = s.Close(); err != nil {
				errs <- fmt.Errorf("closing p2p stream: %w", err)
			}
		}(n.self, receiver)
	}
	wg.Wait()

	close(errs)
	count := 0
	var allErr error
	for err := range errs {
		if err != nil {
			count++
			allErr = errors.Join(allErr, err)
		}
	}
	if count == len(receivers) {
		return fmt.Errorf("send failed: %w", allErr)
	}
	return nil
}

func (n *LibP2PNetwork) sendMsg(ctx context.Context, data []byte, protocolID string, receiverID peer.ID) (rErr error) {
	ctx, span := n.tracer.Start(ctx, "LibP2PNetwork.sendMsg", trace.WithAttributes(attribute.Stringer("receiver", receiverID)))
	defer func() {
		if rErr != nil {
			span.RecordError(rErr)
			span.SetStatus(codes.Error, rErr.Error())
		}
		span.End()
	}()

	s, err := n.self.CreateStream(ctx, receiverID, protocolID)
	if err != nil {
		return fmt.Errorf("open p2p stream: %w", err)
	}
	deadline, _ := ctx.Deadline()
	err = s.SetWriteDeadline(deadline)
	if _, err = s.Write(data); err != nil {
		// on error reset to make sure that the next stream is not affected by the same error
		// reset forces close of both ends of the stream
		if resetErr := s.Reset(); resetErr != nil {
			return errors.Join(fmt.Errorf("writing data to p2p stream: %w", err), resetErr)
		}
		return fmt.Errorf("writing data to p2p stream: %w", err)
	}
	if err = s.Close(); err != nil {
		return fmt.Errorf("closing p2p stream: %w", err)
	}
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
			n.receivedMsg(s.Conn().RemotePeer(), protocolID, msg)
		}
		success = true
	}
}

func (n *LibP2PNetwork) receivedMsg(from peer.ID, protocolID string, msg any) {
	select {
	case n.receivedMsgs <- msg:
	default:
		n.log.Warn(fmt.Sprintf("dropping %s message from %s because of slow consumer", protocolID, from))
	}
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
