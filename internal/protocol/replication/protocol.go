package replication

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/protocol"
	libp2pNetwork "github.com/libp2p/go-libp2p-core/network"
)

var (
	ErrPeerIsNil            = errors.New("peer is nil")
	ErrResponseHandlerIsNil = errors.New("response handler is nil")
	ErrRequestHandlerIsNil  = errors.New("request handler is nil")
	ErrTimout               = errors.New("ledger replication timeout")

	ErrUnknownSystemIdentifier = errors.New("unknown system identifier")
)

const (
	// ProtocolIdLedgerReplication is the protocol.ID of the AlphaBill ledger replication protocol.
	ProtocolIdLedgerReplication = "/ab/ledger/replication/0.0.1"
	requestBufferSize           = 20
)

type (
	Protocol struct {
		self            *network.Peer
		requestHandler  RequestHandler
		responseHandler ResponseHandler
		timeout         time.Duration
	}

	// RequestHandler defines the function to handle the ledger replication requests.
	RequestHandler func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error)

	ResponseHandler func(block *block.Block)
)

// New creates a new ledger replication protocol.
func New(self *network.Peer, timeout time.Duration, requestHandler RequestHandler, responseHandler ResponseHandler) (*Protocol, error) {
	if self == nil {
		return nil, ErrPeerIsNil
	}
	if responseHandler == nil {
		return nil, ErrResponseHandlerIsNil
	}
	if requestHandler == nil {
		return nil, ErrRequestHandlerIsNil
	}
	t := &Protocol{self: self, requestHandler: requestHandler, responseHandler: responseHandler, timeout: timeout}
	self.RegisterProtocolHandler(ProtocolIdLedgerReplication, t.handleStream)
	return t, nil
}

// GetBlocks requests blocks with given system identifier from a random peer in the network. If toBlockNr is 0 then
// response stream contains every block from fromBlockNr till head. It is possible that a reply misses some newer
// blocks.
func (p *Protocol) GetBlocks(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) error {
	if systemIdentifier == nil {
		return errors.New(errstr.NilArgument)
	}
	if toBlockNr != 0 && fromBlockNr > toBlockNr {
		return errors.Errorf("from block nr %v is greater than to block nr %v ", fromBlockNr, toBlockNr)
	}

	var buf bytes.Buffer
	buf.Write(systemIdentifier)
	buf.Write(util.Uint64ToBytes(fromBlockNr))
	buf.Write(util.Uint64ToBytes(toBlockNr))
	request := buf.Bytes()

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	errorCh := make(chan error, 1)
	responseCh := make(chan *LedgerReplicationResponse, 1)
	randomPeer := p.self.GetRandomPeerID()
	logger.Debug("Requesting block from %v, request parameters %X", randomPeer, request)
	go func() {
		defer close(responseCh)
		defer close(errorCh)
		defer cancel()
		s, err := p.self.CreateStream(ctx, randomPeer, ProtocolIdLedgerReplication)
		if err != nil {
			errorCh <- errors.Wrap(err, "failed to open stream")
			return
		}
		defer func() {
			err := s.Close()
			if err != nil {
				logger.Warning("Failed to close libp2p stream: %v", err)
			}
		}()

		_, err = s.Write(request)
		if err != nil {
			errorCh <- errors.Errorf("failed to write request: %v", err)
		}

		reader := protocol.NewProtoBufReader(s)
		defer func() {
			err = reader.Close()
			if err != nil {
				errorCh <- errors.Errorf("failed to close protobuf reader: %v", err)
			}
		}()
		response := &LedgerReplicationResponse{}
		err = reader.Read(response)
		if err != nil {
			errorCh <- errors.Errorf("failed to read response: %v", err)
		}
		responseCh <- response
	}()

	select {
	case <-ctx.Done():
		logger.Info("forwarding timeout")
		return ErrTimout
	case err := <-errorCh:
		logger.Info("Ledger replication failed: %v", err)
		return err
	case response := <-responseCh:
		if response.Status != LedgerReplicationResponse_OK {
			return errors.Errorf("ledger replication request failed. returned status: %v, error message: %v", response.Status, response.Message)
		}
		logger.Info("Received %v block from %v", len(response.Blocks), randomPeer)
		for _, block := range response.Blocks {
			p.responseHandler(block)
		}
		return nil
	}
}

// handleStream receives incoming ledger replication requests from other peers in the network. It reads 20 bytes from
// the stream and expects the following format:
//		* bytes 0...3 - system identifier (4 bytes),
//		* bytes 4...11 - fromBlockNr (8 bytes, uint64 BE encoding), and
//	    * bytes 12...19 - toBlockNr (8 bytes, uint64 BE encoding).
// If request does not contain 20 bytes then an error is returned, otherwise the request is forwarded to RequestHandler.
// Blocks returned by the RequestHandler are written to the output stream. If the request is invalid or the
// RequestHandler call fails then an error response is written to the output stream.
func (p *Protocol) handleStream(s libp2pNetwork.Stream) {
	defer func() {
		err := s.Close()
		if err != nil {
			logger.Warning("Failed to close protobuf reader: %v", err)
		}
	}()
	// system identifier - 4bytes, fromBlock & toBlock 16 bytes = 20 bytes of data
	requestBuffer := make([]byte, requestBufferSize)
	_, err := io.ReadFull(s, requestBuffer)
	if err != nil {
		logger.Info("failed to read request: %v", err)
		writeErrorResponse(LedgerReplicationResponse_INVALID_REQUEST_PARAMETERS, err, s)
		return
	}
	logger.Debug("Raw ledger replication request %X", requestBuffer)
	systemIdentifier := requestBuffer[0:4]
	fromBlockNr := binary.BigEndian.Uint64(requestBuffer[4:12])
	toBlockNr := binary.BigEndian.Uint64(requestBuffer[12:20])
	logger.Debug("Handling ledger replication request: systemIdentifier=%X, fromBlock=%v, toBlock=%v", systemIdentifier, fromBlockNr, toBlockNr)
	if toBlockNr != 0 && fromBlockNr > toBlockNr {
		writeErrorResponse(LedgerReplicationResponse_INVALID_REQUEST_PARAMETERS, errors.Errorf("from block nr %v is greater than to block nr %v ", fromBlockNr, toBlockNr), s)
		return
	}
	blocks, err := p.requestHandler(systemIdentifier, fromBlockNr, toBlockNr)
	if err != nil {
		logger.Warning("failed to handle ledger replication request: %v", err)
		if err == ErrUnknownSystemIdentifier {
			writeErrorResponse(LedgerReplicationResponse_UNKNOWN_SYSTEM_IDENTIFIER, err, s)
			return
		}
		writeErrorResponse(LedgerReplicationResponse_UNKNOWN, err, s)
		return
	}
	logger.Debug("Handler returned %v blocks", len(blocks))
	writer := protocol.NewProtoBufWriter(s)
	defer func() {
		err = writer.Close()
		if err != nil {
			logger.Warning("failed to close protobuf writer: %v", err)
		}
	}()
	err = writer.Write(&LedgerReplicationResponse{
		Status: LedgerReplicationResponse_OK,
		Blocks: blocks,
	})
	if err != nil {
		logger.Warning("Failed to write response: %v", err)
	}
}

func writeErrorResponse(status LedgerReplicationResponse_Status, e error, s libp2pNetwork.Stream) {
	writer := protocol.NewProtoBufWriter(s)
	defer func() {
		err := writer.Close()
		if err != nil {
			logger.Warning("failed to close protobuf writer: %v", err)
		}
	}()
	response := &LedgerReplicationResponse{
		Status:  status,
		Message: e.Error(),
	}
	err := writer.Write(response)
	if err != nil {
		logger.Warning("Failed to write response: %v", err)
	}
}
