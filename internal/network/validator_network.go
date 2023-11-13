package network

import (
	"context"
	"crypto"
	"fmt"
	"log/slog"
	"time"

	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/alphabill-org/alphabill/internal/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/internal/network/protocol/replication"
	"github.com/alphabill-org/alphabill/internal/txbuffer"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	ProtocolInputForward          = "/ab/input-forward/0.0.1"
	ProtocolBlockProposal         = "/ab/block-proposal/0.0.1"
	ProtocolLedgerReplicationReq  = "/ab/replication-req/0.0.1"
	ProtocolLedgerReplicationResp = "/ab/replication-resp/0.0.1"

)

var DefaultValidatorNetworkOptions = ValidatorNetworkOptions{
	ReceivedChannelCapacity:          1000,
	TxBufferSize:                     1000,
	BlockCertificationTimeout:        300 * time.Millisecond,
	BlockProposalTimeout:             300 * time.Millisecond,
	LedgerReplicationRequestTimeout:  300 * time.Millisecond,
	LedgerReplicationResponseTimeout: 300 * time.Millisecond,
	HandshakeTimeout:                 300 * time.Millisecond,
}

type ValidatorNetworkOptions struct {
	// How many messages will be buffered (ReceivedChannel) in case of slow consumer.
	// Once buffer is full messages will be dropped (ie not processed)
	// until consumer catches up.
	ReceivedChannelCapacity uint
	TxBufferSize            uint
	TxBufferHashAlgorithm   crypto.Hash

	// timeout configurations for Send operations.
	// timeout values are per receiver, ie when calling Send with multiple receivers
	// each receiver will have it's own timeout. The context used with Send call can
	// be used to set timeout for whole Send call.

	BlockCertificationTimeout        time.Duration
	BlockProposalTimeout             time.Duration
	LedgerReplicationRequestTimeout  time.Duration
	LedgerReplicationResponseTimeout time.Duration
	HandshakeTimeout                 time.Duration
}

type validatorNetwork struct {
	txBuffer       *txbuffer.TxBuffer
	*LibP2PNetwork
}

type TxProcessor func(tx *types.TransactionOrder) error

/*
NewLibP2PValidatorNetwork creates a new LibP2PNetwork based validator network.

Logger (log) is assumed to already have node_id attribute added, won't be added by NW component!
*/
func NewLibP2PValidatorNetwork(self *Peer, opts ValidatorNetworkOptions, log *slog.Logger) (*validatorNetwork, error) {
	base, err := newLibP2PNetwork(self, opts.ReceivedChannelCapacity, log)
	if err != nil {
		return nil, err
	}

	txBuffer, err := txbuffer.New(opts.TxBufferSize, opts.TxBufferHashAlgorithm, log)
	if err != nil {
		return nil, fmt.Errorf("tx buffer init error, %w", err)
	}

	n := &validatorNetwork{
		txBuffer: txBuffer,
		LibP2PNetwork: base,
	}

	sendProtocolDescriptions := []sendProtocolDescription{
		{protocolID: ProtocolBlockProposal, timeout: opts.BlockProposalTimeout, msgType: blockproposal.BlockProposal{}},
		{protocolID: ProtocolBlockCertification, timeout: opts.BlockCertificationTimeout, msgType: certification.BlockCertificationRequest{}},
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
			handler:    n.handleTransactions,
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

func (n *validatorNetwork) AddTransaction(tx *types.TransactionOrder) ([]byte, error) {
	return n.txBuffer.Add(tx)
}

func (n *validatorNetwork) ProcessTransactions(ctx context.Context, txProcessor TxProcessor) {
	for {
		tx, err := n.txBuffer.Remove(ctx)
		if err != nil {
			n.log.WarnContext(ctx, "getting transaction from buffer", logger.Error(err))
			return
		}
		if err := txProcessor(tx); err != nil {
			n.log.WarnContext(ctx, "processing transaction", logger.Error(err), logger.UnitID(tx.UnitID()))
		}
	}
}

func (n *validatorNetwork) ForwardTransactions(ctx context.Context, receiver peer.ID) {
	var stream libp2pNetwork.Stream
	for {
		tx, err := n.txBuffer.Remove(ctx)
		if err != nil {
			n.log.WarnContext(ctx, "getting transaction from buffer", logger.Error(err))
			return
		}

		msg := fmt.Sprintf("forward tx %X to %v", tx.Hash(n.txBuffer.HashAlgorithm()), receiver)
		n.log.DebugContext(ctx,	msg, logger.UnitID(tx.UnitID()))

		if stream == nil {
			var err error
			stream, err = n.self.CreateStream(ctx, receiver, ProtocolInputForward)
			if err != nil {
				n.log.WarnContext(ctx, "opening p2p stream", logger.Error(err), logger.UnitID(tx.UnitID()))
				return
			}
			defer func() {
				if err := stream.Close(); err != nil {
					n.log.WarnContext(ctx, "opening p2p stream", logger.Error(err), logger.UnitID(tx.UnitID()))
				}
			}()
		}

		data, err := serializeMsg(tx)
		if err != nil {
			n.log.WarnContext(ctx, "serializing tx", logger.Error(err), logger.UnitID(tx.UnitID()))
			continue
		}

		if _, err := stream.Write(data); err != nil {
			n.log.WarnContext(ctx, "writing data to p2p stream", logger.Error(err), logger.UnitID(tx.UnitID()))
			continue
		}
	}
}

func (n *validatorNetwork) handleTransactions(stream libp2pNetwork.Stream) {
	defer func() {
		if err := stream.Close(); err != nil {
			n.log.Warn(fmt.Sprintf("closing p2p stream %q", ProtocolInputForward), logger.Error(err))
		}
	}()

	for {
		tx := &types.TransactionOrder{}
		if err := deserializeMsg(stream, tx); err != nil {
			n.log.Warn(fmt.Sprintf("reading %q message", ProtocolInputForward), logger.Error(err))
			return
		}

		if _, err := n.txBuffer.Add(tx); err != nil {
			n.log.Warn("adding tx to buffer", logger.Error(err))
		}
	}
}
