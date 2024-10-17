package network

import (
	"time"

	"github.com/alphabill-org/alphabill/network/protocol/abdrc"
)

const (
	ProtocolRootIrChangeReq = "/ab/root-change-req/0.0.1"
	ProtocolRootProposal    = "/ab/root-proposal/0.0.1"
	ProtocolRootVote        = "/ab/root-vote/0.0.1"
	ProtocolRootTimeout     = "/ab/root-timeout/0.0.1"
	ProtocolRootStateReq    = "/ab/root-state-req/0.0.1"
	ProtocolRootStateResp   = "/ab/root-state-resp/0.0.1"
)

func NewLibP2RootConsensusNetwork(self *Peer, capacity uint, sendTimeout time.Duration, obs Observability) (*LibP2PNetwork, error) {
	n, err := newLibP2PNetwork(self, capacity, obs)
	if err != nil {
		return nil, err
	}
	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolRootIrChangeReq, timeout: sendTimeout, msgType: abdrc.IrChangeReqMsg{}},
		{protocolID: ProtocolRootProposal, timeout: sendTimeout, msgType: abdrc.ProposalMsg{}},
		{protocolID: ProtocolRootVote, timeout: sendTimeout, msgType: abdrc.VoteMsg{}},
		{protocolID: ProtocolRootTimeout, timeout: sendTimeout, msgType: abdrc.TimeoutMsg{}},
		{protocolID: ProtocolRootStateReq, timeout: sendTimeout, msgType: abdrc.StateRequestMsg{}},
		{protocolID: ProtocolRootStateResp, timeout: sendTimeout, msgType: abdrc.StateMsg{}},
	}
	if err = n.registerSendProtocols(sendProtocolDescriptions); err != nil {
		return nil, err
	}
	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolRootIrChangeReq,
			typeFn:     func() any { return &abdrc.IrChangeReqMsg{} },
		},
		{
			protocolID: ProtocolRootProposal,
			typeFn:     func() any { return &abdrc.ProposalMsg{} },
		},
		{
			protocolID: ProtocolRootVote,
			typeFn:     func() any { return &abdrc.VoteMsg{} },
		},
		{
			protocolID: ProtocolRootTimeout,
			typeFn:     func() any { return &abdrc.TimeoutMsg{} },
		},
		{
			protocolID: ProtocolRootStateReq,
			typeFn:     func() any { return &abdrc.StateRequestMsg{} },
		},
		{
			protocolID: ProtocolRootStateResp,
			typeFn:     func() any { return &abdrc.StateMsg{} },
		},
	}
	if err = n.registerReceiveProtocols(receiveProtocolDescriptions); err != nil {
		return nil, err
	}
	return n, nil
}
