package replication

import (
	"testing"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/libp2p/go-libp2p-core/peerstore"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/testnetwork"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/network"
)

type responseHandler struct {
	receivedBlocks []*block.Block
}

func TestNew_NotOk(t *testing.T) {
	type args struct {
		self            *network.Peer
		timeout         time.Duration
		requestHandler  RequestHandler
		responseHandler ResponseHandler
	}
	tests := []struct {
		name    string
		args    args
		wantErr error
	}{
		{
			name: "self is nil",
			args: args{
				self:    nil,
				timeout: time.Second,
				requestHandler: func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error) {
					return nil, nil
				},
				responseHandler: emptyResponseHandler,
			},
			wantErr: ErrPeerIsNil,
		},
		{
			name: "request handler is nil",
			args: args{
				self:            testnetwork.CreatePeer(t),
				timeout:         time.Second,
				requestHandler:  nil,
				responseHandler: emptyResponseHandler,
			},
			wantErr: ErrRequestHandlerIsNil,
		},
		{
			name: "response handler is nil",
			args: args{self: testnetwork.CreatePeer(t),
				timeout:         time.Second,
				requestHandler:  emptyRequestHandler,
				responseHandler: nil,
			},
			wantErr: ErrResponseHandlerIsNil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(tt.args.self, tt.args.timeout, tt.args.requestHandler, tt.args.responseHandler)
			require.ErrorIs(t, err, tt.wantErr)
			require.Nil(t, got)
		})
	}
}

func TestGetBlocks_Ok(t *testing.T) {
	blocks := &responseHandler{}
	blockReceiverPeer, blockSenderPeer := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, time.Second, emptyRequestHandler, blocks.responseHandler)
	require.NoError(t, err)
	_, err = New(
		blockSenderPeer,
		time.Second,
		func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error) {
			b := &block.Block{
				SystemIdentifier: systemIdentifier,
				BlockNumber:      0,
			}
			return []*block.Block{b}, nil
		},
		emptyResponseHandler,
	)
	require.NoError(t, err)

	err = receiverProtocol.GetBlocks([]byte{0, 0, 0, 0}, 1, 2)
	require.Eventually(t, func() bool {
		return len(blocks.receivedBlocks) == 1
	}, test.WaitDuration, test.WaitTick)
	require.NoError(t, err)
}

func TestGetBlocks_Timeout(t *testing.T) {
	blockReceiverPeer, blockSenderPeer := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, 20*time.Millisecond, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	_, err = New(
		blockSenderPeer,
		20*time.Millisecond,
		func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error) {
			time.Sleep(10 * time.Second)
			return nil, nil
		},
		emptyResponseHandler,
	)
	require.NoError(t, err)

	err = receiverProtocol.GetBlocks([]byte{0, 0, 0, 0}, 1, 2)
	require.ErrorIs(t, err, ErrTimout)
}

func TestGetBlocks_SystemIdentifierIsNil(t *testing.T) {
	blockReceiverPeer, _ := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, time.Second, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	err = receiverProtocol.GetBlocks(nil, 2, 5)
	require.ErrorContains(t, err, errstr.NilArgument)
}

func TestGetBlocks_ErrUnknownSystemIdentifier(t *testing.T) {
	blockReceiverPeer, blockSenderPeer := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, time.Second, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	_, err = New(
		blockSenderPeer,
		time.Second,
		func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error) {
			return nil, ErrUnknownSystemIdentifier
		},
		emptyResponseHandler,
	)
	require.NoError(t, err)
	err = receiverProtocol.GetBlocks([]byte{0, 0, 0, 0}, 2, 4)
	require.ErrorContains(t, err, "ledger replication request failed. returned status: UNKNOWN_SYSTEM_IDENTIFIER, error message: unknown system identifier")
}

func TestGetBlocks_RequestHandlerReturnsUnknownError(t *testing.T) {
	blockReceiverPeer, blockSenderPeer := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, time.Second, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	r := errors.New("test")
	_, err = New(
		blockSenderPeer,
		time.Second,
		func(systemIdentifier []byte, fromBlockNr, toBlockNr uint64) ([]*block.Block, error) {
			return nil, r
		},
		emptyResponseHandler,
	)
	require.NoError(t, err)
	err = receiverProtocol.GetBlocks([]byte{0, 0, 0, 0}, 2, 4)
	require.ErrorContains(t, err, "ledger replication request failed. returned status: UNKNOWN, error message: test")
}

func TestGetBlocks_InvalidFromBlockNrAndToBlockNr(t *testing.T) {
	blockReceiverPeer, blockSenderPeer := createPeers(t)
	receiverProtocol, err := New(blockReceiverPeer, time.Second, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	_, err = New(blockSenderPeer, time.Second, emptyRequestHandler, emptyResponseHandler)
	require.NoError(t, err)
	err = receiverProtocol.GetBlocks([]byte{0, 0, 0, 0}, 4, 2)
	require.ErrorContains(t, err, "from block nr 4 is greater than to block nr 2")
}

func emptyResponseHandler(*block.Block) {
}

func createPeers(t *testing.T) (*network.Peer, *network.Peer) {
	blockReceiverPeer := testnetwork.CreatePeer(t)
	blockSenderPeer := testnetwork.CreatePeer(t)
	blockReceiverPeer.Network().Peerstore().AddAddrs(blockSenderPeer.ID(), blockSenderPeer.MultiAddresses(), peerstore.PermanentAddrTTL)
	blockSenderPeer.Network().Peerstore().AddAddrs(blockReceiverPeer.ID(), blockReceiverPeer.MultiAddresses(), peerstore.PermanentAddrTTL)
	return blockReceiverPeer, blockSenderPeer
}

func emptyRequestHandler([]byte, uint64, uint64) ([]*block.Block, error) {
	return nil, nil
}

func (r *responseHandler) responseHandler(b *block.Block) {
	r.receivedBlocks = append(r.receivedBlocks, b)
}
