package network

import (
	"fmt"
	"time"

	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	ProtocolBlockProposal         = "/ab/block-proposal/0.0.1"
	ProtocolUnicityCertificates   = "/ab/certificates/0.0.1"
	ProtocolBlockCertification    = "/ab/block-certification/0.0.1"
	ProtocolInputForward          = "/ab/input-forward/0.0.1"
	ProtocolLedgerReplicationReq  = "/ab/replication-req/0.0.1"
	ProtocolLedgerReplicationResp = "/ab/replication-resp/0.0.1"
	ProtocolHandshake             = "/ab/handshake/0.0.1"
)

var DefaultValidatorNetOptions = ValidatorNetOptions{
	ResponseChannelCapacity:          1000,
	ForwarderTimeout:                 300 * time.Millisecond,
	BlockCertificationTimeout:        300 * time.Millisecond,
	BlockProposalTimeout:             300 * time.Millisecond,
	LedgerReplicationRequestTimeout:  300 * time.Millisecond,
	LedgerReplicationResponseTimeout: 300 * time.Millisecond,
	HandshakeTimeout:                 300 * time.Millisecond,
}

type ValidatorNetOptions struct {
	// How many messages will be buffered (ReceivedChannel) in case of slow consumer.
	// Once buffer is full messages will be dropped (ie not processed)
	// until consumer catches up.
	ResponseChannelCapacity uint

	// timeout configurations for Send operations.
	// timeout values are per receiver, ie when calling Send with multiple receivers
	// each receiver will have it's own timeout. The context used with Send call can
	// be used to set timeout for whole Send call.

	ForwarderTimeout                 time.Duration
	BlockCertificationTimeout        time.Duration
	BlockProposalTimeout             time.Duration
	LedgerReplicationRequestTimeout  time.Duration
	LedgerReplicationResponseTimeout time.Duration
	HandshakeTimeout                 time.Duration
}

// NewLibP2PValidatorNetwork creates a new libp2p for a validator.
func NewLibP2PValidatorNetwork(self *Peer, opts ValidatorNetOptions) (*LibP2PNetwork, error) {
	n, err := newLibP2PNetwork(self, opts.ResponseChannelCapacity)
	if err != nil {
		return nil, err
	}

	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolBlockProposal, timeout: opts.BlockProposalTimeout, msgType: blockproposal.BlockProposal{}},
		{protocolID: ProtocolBlockCertification, timeout: opts.BlockCertificationTimeout, msgType: certification.BlockCertificationRequest{}},
		{protocolID: ProtocolInputForward, timeout: opts.ForwarderTimeout, msgType: types.TransactionOrder{}},
		{protocolID: ProtocolLedgerReplicationReq, timeout: opts.LedgerReplicationRequestTimeout, msgType: replication.LedgerReplicationRequest{}},
		{protocolID: ProtocolLedgerReplicationResp, timeout: opts.LedgerReplicationResponseTimeout, msgType: replication.LedgerReplicationResponse{}},
		{protocolID: ProtocolHandshake, timeout: opts.HandshakeTimeout, msgType: handshake.Handshake{}},
	}
	if err = n.registerSendProtocols(sendProtocolDescriptions); err != nil {
		return nil, err
	}

	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolBlockProposal,
			typeFn:     func() any { return &blockproposal.BlockProposal{} },
		},
		{
			protocolID: ProtocolInputForward,
			typeFn:     func() any { return &types.TransactionOrder{} },
		},
		{
			protocolID: ProtocolUnicityCertificates,
			typeFn:     func() any { return &types.UnicityCertificate{} },
		},
		{
			protocolID: ProtocolLedgerReplicationReq,
			typeFn:     func() any { return &replication.LedgerReplicationRequest{} },
		},
		{
			protocolID: ProtocolLedgerReplicationResp,
			typeFn:     func() any { return &replication.LedgerReplicationResponse{} },
		},
	}
	if err = n.registerReceiveProtocols(receiveProtocolDescriptions); err != nil {
		return nil, err
	}

	return n, nil
}

func NewLibP2PRootChainNetwork(self *Peer, capacity uint, sendCertificateTimeout time.Duration) (*LibP2PNetwork, error) {
	n, err := newLibP2PNetwork(self, capacity)
	if err != nil {
		return nil, err
	}

	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolUnicityCertificates, timeout: sendCertificateTimeout, msgType: types.UnicityCertificate{}},
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
