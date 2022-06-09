package partition

import (
	"time"

	uc "gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/blockproposal"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/certification"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol/forwarder"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"google.golang.org/protobuf/proto"
)

var DefaultPartitionNetOptions = NetOptions{
	ResponseChannelCapacity:   1000,
	ForwarderTimeout:          300 * time.Millisecond,
	BlockCertificationTimeout: 300 * time.Millisecond,
	BlockProposalTimeout:      300 * time.Millisecond,
}

type NetOptions struct {
	ResponseChannelCapacity   uint
	ForwarderTimeout          time.Duration
	BlockCertificationTimeout time.Duration
	BlockProposalTimeout      time.Duration
}

func NewPartitionNetwork(self *network.Peer, opts NetOptions) (*network.LibP2PNetwork, error) {
	if self == nil {
		return nil, errors.New(errstr.NilArgument)
	}
	n, err := network.NewLibP2PNetwork(self, opts.ResponseChannelCapacity)
	if err != nil {
		return nil, err
	}

	err = initSendProtocols(self, opts, n)
	if err != nil {
		return nil, err
	}
	err = initReceiveProtocols(self, n)
	if err != nil {
		return nil, err
	}
	return n, nil
}

func initReceiveProtocols(self *network.Peer, n *network.LibP2PNetwork) error {
	receiveProtocolDescriptions := []struct {
		protocolID string
		typeFn     protocol.TypeFunc[proto.Message]
	}{
		{
			protocolID: blockproposal.ProtocolBlockProposal,
			typeFn: func() proto.Message {
				return &blockproposal.BlockProposal{}
			},
		},
		{
			protocolID: forwarder.ProtocolInputForward,
			typeFn: func() proto.Message {
				return &txsystem.Transaction{}
			},
		},
		{
			protocolID: certificates.ProtocolReceiveUnicityCertificate,
			typeFn: func() proto.Message {
				return &uc.UnicityCertificate{}
			},
		},
	}

	for _, d := range receiveProtocolDescriptions {
		err := initReceiveProtocol(self, d.protocolID, d.typeFn, n)
		if err != nil {
			return err
		}
	}
	/*
		p, err := protocol.NewReceiverProtocol[*blockproposal.BlockProposal](self, blockproposal.ProtocolBlockProposal, n.ReceivedMsgCh, func() *blockproposal.BlockProposal { return &blockproposal.BlockProposal{} })
		if err != nil {
			return err
		}
		err = n.RegisterReceiveProtocol(p)
		if err != nil {
			return err
		}*/
	return nil
}

func initReceiveProtocol(self *network.Peer, protocolID string, typeFn protocol.TypeFunc[proto.Message], n *network.LibP2PNetwork) error {
	p, err := protocol.NewReceiverProtocol(self, protocolID, n.ReceivedMsgCh, typeFn)
	if err != nil {
		return err
	}
	err = n.RegisterReceiveProtocol(p)
	if err != nil {
		return err
	}
	return nil
}

func initSendProtocols(self *network.Peer, opts NetOptions, n *network.LibP2PNetwork) error {
	sendProtocolDescriptions := []struct {
		protocolID string
		timeout    time.Duration
	}{
		{protocolID: blockproposal.ProtocolBlockProposal, timeout: opts.BlockProposalTimeout},
		{protocolID: certification.ProtocolBlockCertification, timeout: opts.BlockCertificationTimeout},
		{protocolID: forwarder.ProtocolInputForward, timeout: opts.ForwarderTimeout},
	}
	for _, pd := range sendProtocolDescriptions {
		err := initSendProtocol(pd.protocolID, self, pd.timeout, n)
		if err != nil {
			return err
		}
	}
	return nil
}

func initSendProtocol(protocolID string, peer *network.Peer, timeout time.Duration, n *network.LibP2PNetwork) error {
	p, err := protocol.NewSendProtocol(peer, protocolID, timeout)
	if err != nil {
		return err
	}
	if err = n.RegisterSendProtocol(p); err != nil {
		return errors.Wrapf(err, "failed to register protocol %s ", protocolID)
	}
	return nil
}

/*
func initBlockProposalProtocol(self *network.Peer, opts NetOptions, n *network.LibP2PNetwork) error {
	p, err := blockproposal.New(self, opts.BlockProposalTimeout, n.ReceivedMsgCh)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize %s protocol", blockproposal.ProtocolBlockProposal)
	}
	if err = n.RegisterReceiveProtocol(p); err != nil {
		return errors.Wrapf(err, "failed to register receive protocol %s ", blockproposal.ProtocolBlockProposal)
	}
	if err = n.RegisterSendProtocol(p); err != nil {
		return errors.Wrapf(err, "failed to register protocol %s ", blockproposal.ProtocolBlockProposal)
	}
	return nil
}

// initForwardingProtocol initialises transaction forwarding protocol. Partition node is sender and receiver.
func initForwardingProtocol(self *network.Peer, opts NetOptions, n *network.LibP2PNetwork) error {
	f, err := forwarder.New(self, opts.ForwarderTimeout, n.ReceivedMsgCh)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize %s protocol", forwarder.ProtocolInputForward)
	}
	if err = n.RegisterReceiveProtocol(f); err != nil {
		return errors.Wrapf(err, "failed to register receive protocol %s ", forwarder.ProtocolInputForward)
	}
	if err = n.RegisterSendProtocol(f); err != nil {
		return errors.Wrapf(err, "failed to register protocol %s ", forwarder.ProtocolInputForward)
	}
	return nil
}

// initUnicityCertificateProtocol initialises unicity certificate protocol. Partition node is receiver.
func initUnicityCertificateProtocol(self *network.Peer, n *network.LibP2PNetwork) error {
	p, err := certificates.NewReceiverProtocol(self, n.ReceivedMsgCh)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize %s protocol", certificates.ProtocolReceiveUnicityCertificate)
	}
	if err = n.RegisterReceiveProtocol(p); err != nil {
		return errors.Wrapf(err, "failed to register receive protocol %s ", certificates.ProtocolReceiveUnicityCertificate)
	}
	return nil
}

func initBlockCertificationProtocol(self *network.Peer, opts NetOptions, n *network.LibP2PNetwork) error {
	p, err := certification.NewSenderProtocol(self, opts.BlockCertificationTimeout)
	if err != nil {
		return errors.Wrapf(err, "failed to initialize %s protocol", certification.ProtocolBlockCertification)
	}
	if err = n.RegisterSendProtocol(p); err != nil {
		return errors.Wrapf(err, "failed to register receive protocol %s ", certification.ProtocolBlockCertification)
	}
	return nil
}*/
