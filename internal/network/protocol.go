package network

import (
	"time"

	"google.golang.org/protobuf/proto"
)

const (
	ProtocolBlockProposal       = "/ab/block-proposal/0.0.1"
	ProtocolUnicityCertificates = "/ab/certificates/0.0.1"
	ProtocolBlockCertification  = "/ab/block-certification/0.0.1"
	ProtocolInputForward        = "/ab/input-forward/0.0.1"
	//ProtocolLedgerReplication   = "/ab/replication/0.0.1"

	ErrStrPeerIsNil       = "peer is nil"
	ErrStrProtocolIDEmpty = "protocol ID is empty"
	ErrStrOutputChIsNil   = "output channel is nil"
	ErrStrTypeFuncIsNil   = "type func is nil"
)

type (
	protocol struct {
		self       *Peer
		protocolID string
	}

	// SendProtocol is used to send protobuf messages to other peers in the network.
	SendProtocol struct {
		*protocol
		timeout time.Duration
	}

	// ReceiveProtocol is used to receive protobuf messages from other peers in the network.
	ReceiveProtocol[T proto.Message] struct {
		*protocol
		outCh    chan<- ReceivedMessage
		typeFunc TypeFunc[T]
	}

	// TypeFunc creates a new instance of protobuf message.
	TypeFunc[T proto.Message] func() T
)
