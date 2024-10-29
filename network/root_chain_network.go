package network

import (
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
)

const (
	ProtocolHandshake           = "/ab/handshake/0.0.1"
	ProtocolBlockCertification  = "/ab/block-certification/0.0.1"
	ProtocolUnicityCertificates = "/ab/certificates/0.0.1"
)

/*
Logger (log) is assumed to already have node_id attribute added, won't be added by NW component!
*/
func NewLibP2PRootChainNetwork(self *Peer, capacity uint, sendCertificateTimeout time.Duration, obs Observability) (*LibP2PNetwork, error) {
	n, err := newLibP2PNetwork(self, capacity, obs)
	if err != nil {
		return nil, err
	}

	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolUnicityCertificates, timeout: sendCertificateTimeout, msgType: certification.CertificationResponse{}},
	}
	if err = n.registerSendProtocols(sendProtocolDescriptions); err != nil {
		return nil, fmt.Errorf("registering send protocols: %w", err)
	}

	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolBlockCertification,
			typeFn:     func() any { return &certification.BlockCertificationRequest{} },
		},
		{
			protocolID: ProtocolHandshake,
			typeFn:     func() any { return &handshake.Handshake{} },
		},
	}
	if err = n.registerReceiveProtocols(receiveProtocolDescriptions); err != nil {
		return nil, fmt.Errorf("registering receive protocols: %w", err)
	}

	return n, nil
}
