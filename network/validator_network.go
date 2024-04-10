package network

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/network/protocol/blockproposal"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/network/protocol/handshake"
	"github.com/alphabill-org/alphabill/network/protocol/replication"
	"github.com/alphabill-org/alphabill/txbuffer"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	libp2pNetwork "github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	ProtocolInputForward          = "/ab/input-forward/0.0.1"
	ProtocolBlockProposal         = "/ab/block-proposal/0.0.1"
	ProtocolLedgerReplicationReq  = "/ab/replication-req/0.0.1"
	ProtocolLedgerReplicationResp = "/ab/replication-resp/0.0.1"
	TopicPrefixBlock              = "/ab/block/0.0.1/"
)

var DefaultValidatorNetworkOptions = ValidatorNetworkOptions{
	ReceivedChannelCapacity:          1000,
	TxBufferSize:                     1000,
	TxBufferHashAlgorithm:            crypto.SHA256,
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
	*LibP2PNetwork
	txBuffer *txbuffer.TxBuffer
	txFwdBy  metric.Int64Counter
	txFwdTo  metric.Int64Counter

	gsTopicBlock        *pubsub.Topic
	gsSubscriptionBlock *pubsub.Subscription
}

type TxProcessor func(ctx context.Context, tx *types.TransactionOrder) error

type TxReceiver func() peer.ID

/*
NewLibP2PValidatorNetwork creates a new LibP2PNetwork based validator network.

Logger (log) is assumed to already have node_id attribute added, won't be added by NW component!
*/
func NewLibP2PValidatorNetwork(ctx context.Context, systemID types.SystemID, self *Peer, opts ValidatorNetworkOptions, obs Observability) (*validatorNetwork, error) {
	base, err := newLibP2PNetwork(self, opts.ReceivedChannelCapacity, obs)
	if err != nil {
		return nil, err
	}

	txBuffer, err := txbuffer.New(opts.TxBufferSize, opts.TxBufferHashAlgorithm, obs)
	if err != nil {
		return nil, fmt.Errorf("tx buffer init error, %w", err)
	}

	n := &validatorNetwork{
		LibP2PNetwork: base,
		txBuffer:      txBuffer,
	}

	if err := n.initGossipSub(ctx, systemID); err != nil {
		return nil, fmt.Errorf("initializing gossip protocol: %w", err)
	}

	sendProtocolDescriptions := []sendProtocolDescription{
		{
			protocolID: ProtocolLedgerReplicationReq,
			timeout:    opts.LedgerReplicationRequestTimeout,
			msgType:    replication.LedgerReplicationRequest{}},
		{
			protocolID: ProtocolLedgerReplicationResp,
			timeout:    opts.LedgerReplicationResponseTimeout,
			msgType:    replication.LedgerReplicationResponse{},
		},
	}
	if self.IsValidator() {
		sendProtocolDescriptions = append(sendProtocolDescriptions,
			sendProtocolDescription{
				protocolID: ProtocolBlockProposal,
				timeout:    opts.BlockProposalTimeout,
				msgType:    blockproposal.BlockProposal{},
			},
			sendProtocolDescription{
				protocolID: ProtocolBlockCertification,
				timeout:    opts.BlockCertificationTimeout,
				msgType:    certification.BlockCertificationRequest{},
			},
			sendProtocolDescription{
				protocolID: ProtocolHandshake,
				timeout:    opts.HandshakeTimeout,
				msgType:    handshake.Handshake{},
			},
		)
	}
	if err = n.registerSendProtocols(sendProtocolDescriptions); err != nil {
		return nil, fmt.Errorf("registering send protocols: %w", err)
	}

	receiveProtocolDescriptions := []receiveProtocolDescription{
		{
			protocolID: ProtocolLedgerReplicationReq,
			typeFn:     func() any { return &replication.LedgerReplicationRequest{} },
		},
		{
			protocolID: ProtocolLedgerReplicationResp,
			typeFn:     func() any { return &replication.LedgerReplicationResponse{} },
		},
	}
	if self.IsValidator() {
		receiveProtocolDescriptions = append(receiveProtocolDescriptions,
			receiveProtocolDescription{
				protocolID: ProtocolBlockProposal,
				typeFn:     func() any { return &blockproposal.BlockProposal{} },
			},
			receiveProtocolDescription{
				protocolID: ProtocolInputForward,
				handler:    n.handleTransactions,
			},
			receiveProtocolDescription{
				protocolID: ProtocolUnicityCertificates,
				typeFn:     func() any { return &types.UnicityCertificate{} },
			},
		)
	}
	if err = n.registerReceiveProtocols(receiveProtocolDescriptions); err != nil {
		return nil, fmt.Errorf("registering receive protocols: %w", err)
	}

	if err := n.initMetrics(obs); err != nil {
		return nil, fmt.Errorf("initializing metrics: %w", err)
	}

	return n, nil
}

func (n *validatorNetwork) initGossipSub(ctx context.Context, systemID types.SystemID) error {
	gs, err := pubsub.NewGossipSub(ctx, n.self.host)
	if err != nil {
		return err
	}

	topic := TopicPrefixBlock + systemID.String()
	n.log.InfoContext(ctx, fmt.Sprintf("Joining gossipsub topic %s", topic))

	// Both validators and non-validators join the topic mesh. It should be possible though
	// for validators not to join the mesh and just publish to the topic (fan-out).
	n.gsTopicBlock, err = gs.Join(topic, pubsub.WithTopicMessageIdFn(
		func(msg *pubsub_pb.Message) string {
			hasher := crypto.SHA256.New()
			hasher.Write(msg.Data)
			return hex.EncodeToString(hasher.Sum(nil))
		}),
	)
	if err != nil {
		return err
	}

	// Only non-validators subscribe to receive new blocks
	if !n.self.IsValidator() {
		n.log.InfoContext(ctx, fmt.Sprintf("Subscribing to gossipsub topic %s", topic))
		n.gsSubscriptionBlock, err = n.gsTopicBlock.Subscribe()
		if err != nil {
			return fmt.Errorf("subscribing to topic '%s': %w", topic, err)
		}
		go n.handleBlocks(ctx)
	}
	return nil
}

func (n *validatorNetwork) initMetrics(obs Observability) (err error) {
	m := obs.Meter("partition.node.network")

	if n.txFwdTo, err = m.Int64Counter(
		"fwd.to.tx.count",
		metric.WithDescription("Number of transactions forwarded to the node by peers"),
		metric.WithUnit("{transaction}"),
	); err != nil {
		return fmt.Errorf("creating counter for forwarded tx: %w", err)
	}

	if n.txFwdBy, err = m.Int64Counter(
		"fwd.by.tx.count",
		metric.WithDescription("Number of transactions forwarded by the node (to the leader)"),
		metric.WithUnit("{transaction}"),
	); err != nil {
		return fmt.Errorf("creating counter for forwarded tx: %w", err)
	}

	return nil
}

func (n *validatorNetwork) AddTransaction(ctx context.Context, tx *types.TransactionOrder) ([]byte, error) {
	return n.txBuffer.Add(ctx, tx)
}

func (n *validatorNetwork) PublishBlock(ctx context.Context, block *types.Block) error {
	blockBytes, err := cbor.Marshal(block)
	if err != nil {
		return err
	}

	return n.gsTopicBlock.Publish(ctx, blockBytes)
}

func (n *validatorNetwork) ProcessTransactions(ctx context.Context, txProcessor TxProcessor) {
	ctx, span := n.tracer.Start(ctx, "validatorNetwork.ProcessTransactions")
	defer span.End()
	for {
		tx, err := n.txBuffer.Remove(ctx)
		if err != nil {
			// context cancelled, no need to log
			return
		}
		if err := txProcessor(ctx, tx); err != nil {
			n.log.WarnContext(ctx, "processing transaction", logger.Error(err), logger.UnitID(tx.UnitID()))
		}
	}
}

func (n *validatorNetwork) ForwardTransactions(ctx context.Context, receiverFunc TxReceiver) {
	receiver := receiverFunc()
	if n.self.IsValidator() {
		var span trace.Span
		ctx, span = n.tracer.Start(ctx, "validatorNetwork.ForwardTransactions",
			trace.WithAttributes(attribute.Stringer("receiver", receiver)))
		defer span.End()
	}

	var openStreams []libp2pNetwork.Stream
	defer func() {
		for _, s := range openStreams {
			if err := s.Close(); err != nil {
				n.log.WarnContext(ctx, "closing p2p stream", logger.Error(err))
			}
		}
	}()

	var stream libp2pNetwork.Stream
	for {
		tx, err := n.txBuffer.Remove(ctx)
		if err != nil {
			// context cancelled, no need to log
			return
		}

		addToMetric := func(status string) {
			n.txFwdBy.Add(ctx, 1, metric.WithAttributeSet(
				attribute.NewSet(attribute.String("tx", tx.PayloadType()), attribute.String("status", status))))
		}

		curReceiver := receiverFunc()
		if stream == nil || curReceiver != receiver {
			// Receiver has changed, close the stream to previous receiver.
			if stream != nil {
				if err := stream.Close(); err != nil {
					n.log.WarnContext(ctx, "closing p2p stream", logger.Error(err))
				}
				openStreams = openStreams[:len(openStreams)-1]
			}
			receiver = curReceiver

			var err error
			stream, err = n.self.CreateStream(ctx, receiver, ProtocolInputForward)
			if err != nil {
				n.log.WarnContext(ctx, "opening p2p stream", logger.Error(err), logger.UnitID(tx.UnitID()))
				addToMetric("err")
				return
			}
			openStreams = append(openStreams, stream)
		}

		n.log.DebugContext(ctx,
			fmt.Sprintf("forward tx %X to %v", tx.Hash(n.txBuffer.HashAlgorithm()), receiver),
			logger.UnitID(tx.UnitID()))

		data, err := serializeMsg(tx)
		if err != nil {
			n.log.WarnContext(ctx, "serializing tx", logger.Error(err), logger.UnitID(tx.UnitID()))
			addToMetric("err.serialize")
			continue
		}

		if _, err := stream.Write(data); err != nil {
			n.log.WarnContext(ctx, "writing data to p2p stream", logger.Error(err), logger.UnitID(tx.UnitID()))
			addToMetric("err.write")
			continue
		}

		addToMetric("ok")
	}
}

func (n *validatorNetwork) handleTransactions(stream libp2pNetwork.Stream) {
	ctx, span := n.tracer.Start(context.Background(), "validatorNetwork.handleTransactions")
	defer func() {
		if err := stream.Close(); err != nil {
			n.log.WarnContext(ctx, fmt.Sprintf("closing p2p stream %q", stream.Protocol()), logger.Error(err))
		}
		span.End()
	}()

	for {
		tx := &types.TransactionOrder{}
		if err := deserializeMsg(stream, tx); err != nil {
			if !errors.Is(err, io.EOF) {
				n.log.WarnContext(ctx, fmt.Sprintf("reading %q message", stream.Protocol()), logger.Error(err))
			}
			return
		}

		_, err := n.txBuffer.Add(ctx, tx)
		if err != nil {
			n.log.WarnContext(ctx, "adding tx to buffer", logger.Error(err))
			span.AddEvent(err.Error())
		}

		n.txFwdTo.Add(ctx, 1, metric.WithAttributes(
			attribute.String("tx", tx.PayloadType()),
			attribute.String("status", statusCodeOfTxBufferError(err))))
	}
}

func (n *validatorNetwork) handleBlocks(ctx context.Context) {
	for {
		msg, err := n.gsSubscriptionBlock.Next(ctx)
		if err != nil {
			n.log.WarnContext(ctx, "getting block from topic", logger.Error(err))
			continue
		}

		block := &types.Block{}
		err = cbor.Unmarshal(msg.Data, block)
		if err != nil {
			n.log.WarnContext(ctx, "failed to decode block", logger.Error(err))
			continue
		}

		if err = n.receivedMsg(msg.ReceivedFrom, msg.GetTopic(), block); err != nil {
			n.log.WarnContext(ctx, "failed to receive block", logger.Error(err))
		}
	}
}

func statusCodeOfTxBufferError(err error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, txbuffer.ErrTxIsNil):
		return "nil"
	case errors.Is(err, txbuffer.ErrTxInBuffer):
		return "buf.double"
	case errors.Is(err, txbuffer.ErrTxBufferFull):
		return "buf.full"
	default:
		return "err"
	}
}
