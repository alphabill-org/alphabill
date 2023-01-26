package network

import (
	"errors"

	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"google.golang.org/protobuf/proto"
)

func NewReceiverProtocol[T proto.Message](self *Peer, protocolID string, outCh chan<- ReceivedMessage, typeFunc TypeFunc[T]) (*ReceiveProtocol[T], error) {
	if self == nil {
		return nil, errors.New(ErrStrPeerIsNil)
	}
	if protocolID == "" {
		return nil, errors.New(ErrStrProtocolIDEmpty)
	}
	if outCh == nil {
		return nil, errors.New(ErrStrOutputChIsNil)
	}
	if typeFunc == nil {
		return nil, errors.New(ErrStrTypeFuncIsNil)
	}
	p := &ReceiveProtocol[T]{
		protocol: &protocol{self: self, protocolID: protocolID},
		outCh:    outCh,
		typeFunc: typeFunc,
	}
	self.RegisterProtocolHandler(protocolID, p.HandleStream)
	return p, nil
}

func (p *ReceiveProtocol[T]) ID() string {
	return p.protocolID
}

func (p *ReceiveProtocol[T]) HandleStream(s libp2pNetwork.Stream) {
	r := NewProtoBufReader(s)
	defer func() {
		err := s.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf reader: %v", err)
		}
	}()
	t := p.typeFunc()
	err := r.Read(t)
	if err != nil {
		logger.Warning("Failed to read message: %v", err)
		return
	}
	p.outCh <- ReceivedMessage{
		From:     s.Conn().RemotePeer(),
		Protocol: p.protocolID,
		Message:  t,
	}
}

func (p *ReceiveProtocol[T]) Close() {
	p.self.RemoveProtocolHandler(p.protocolID)
}
