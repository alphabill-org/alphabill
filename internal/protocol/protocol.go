package protocol

import "gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"

const (
	ErrStrPeerIsNil       = "peer is nil"
	ErrStrProtocolIDEmpty = "protocol ID is empty"
	ErrStrOutputChIsNil   = "output channel is nil"
)

type protocol struct {
	self       *network.Peer
	protocolID string
}
