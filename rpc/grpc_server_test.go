package rpc

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

var failingUnitID = types.NewUnitID(33, nil, []byte{5}, []byte{1})

type (
	MockNode struct {
		maxBlockNumber uint64
		maxRoundNumber uint64
		transactions   []*types.TransactionOrder
		err            error
		txs            txsystem.TransactionSystem
	}

	MockOwnerIndex struct {
		err        error
		ownerUnits map[string][]types.UnitID
	}
)

func (mn *MockNode) TransactionSystemState() txsystem.StateReader {
	return mn.txs.State()
}

func (mn *MockNode) GetTransactionRecord(_ context.Context, hash []byte) (*types.TransactionRecord, *types.TxProof, error) {
	if mn.err != nil {
		return nil, nil, mn.err
	}
	return &types.TransactionRecord{}, &types.TxProof{}, nil
}

func (mn *MockNode) SubmitTx(_ context.Context, tx *types.TransactionOrder) ([]byte, error) {
	if bytes.Equal(tx.UnitID(), failingUnitID) {
		return nil, errors.New("failed")
	}
	if tx != nil {
		mn.transactions = append(mn.transactions, tx)
	}
	return tx.Hash(crypto.SHA256), nil
}

func (mn *MockNode) GetBlock(_ context.Context, blockNumber uint64) (*types.Block, error) {
	if mn.err != nil {
		return nil, mn.err
	}
	if blockNumber > mn.maxBlockNumber {
		// empty block
		return nil, nil
	}
	return &types.Block{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: blockNumber}}}, nil
}

func (mn *MockNode) LatestBlockNumber() (uint64, error) {
	return mn.maxBlockNumber, nil
}

func (mn *MockNode) GetLatestRoundNumber(_ context.Context) (uint64, error) {
	if mn.err != nil {
		return 0, mn.err
	}
	return mn.maxRoundNumber, nil
}

func (mn *MockNode) SystemIdentifier() types.SystemID {
	return 0x00010000
}

func (mn *MockNode) GetPeer() *network.Peer {
	return nil
}

func (mn *MockNode) SerializeState(writer io.Writer) error {
	return nil
}

func (mn *MockOwnerIndex) GetOwnerUnits(ownerID []byte) ([]types.UnitID, error) {
	if mn.err != nil {
		return nil, mn.err
	}
	return mn.ownerUnits[string(ownerID)], nil
}

func TestNewRpcServer_PartitionNodeMissing(t *testing.T) {
	p, err := NewGRPCServer(nil, nil)
	assert.Nil(t, p)
	assert.NotNil(t, err)
	assert.EqualError(t, err, `partition node which implements the service must be assigned`)
}

func TestNewRpcServer_Ok(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{}, observability.Default(t))
	assert.NotNil(t, p)
	assert.Nil(t, err)
}

func TestRpcServer_GetBlocksOk(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 12, maxRoundNumber: 12}, observability.Default(t))
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 12})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 12)
	require.EqualValues(t, 12, res.MaxBlockNumber)
}

func TestRpcServer_GetBlocksSingleBlock(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 1, maxRoundNumber: 1}, observability.Default(t))
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 1})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 1)
	require.EqualValues(t, 1, res.MaxBlockNumber)
}

func TestRpcServer_GetBlocksMostlyEmpty(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 5, maxRoundNumber: 100}, observability.Default(t))
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 1, BlockCount: 50})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 5)
	// BatchMaxBlockNumber is really BatchMaxRoundNumber or BatchEnd, including empty blocks,
	// such that next request from client should be BatchEnd + 1.
	require.EqualValues(t, 50, res.BatchMaxBlockNumber)
}

func TestRpcServer_FetchNonExistentBlocks_DoesNotPanic(t *testing.T) {
	p, err := NewGRPCServer(&MockNode{maxBlockNumber: 7, maxRoundNumber: 7}, observability.Default(t))
	require.NoError(t, err)
	res, err := p.GetBlocks(context.Background(), &alphabill.GetBlocksRequest{BlockNumber: 73, BlockCount: 100})
	require.NoError(t, err)
	require.Len(t, res.Blocks, 0)
	require.EqualValues(t, 7, res.MaxBlockNumber)
}

func TestRpcServer_ProcessTransaction_Fails(t *testing.T) {
	ctx := context.Background()
	con, client := createRpcClient(t, ctx)
	defer con.Close()
	response, err := client.ProcessTransaction(ctx, &alphabill.Transaction{Order: createTransactionOrder(t, failingUnitID)})
	assert.Nil(t, response)
	assert.Errorf(t, err, "failed")
}

func TestRpcServer_ProcessTransaction_Ok(t *testing.T) {
	ctx := context.Background()
	con, client := createRpcClient(t, ctx)
	defer con.Close()

	req := createTransactionOrder(t, types.NewUnitID(33, nil, []byte{1}, []byte{1}))
	response, err := client.ProcessTransaction(ctx, &alphabill.Transaction{Order: req})
	assert.Nil(t, err)
	assert.NotNil(t, response)
}

func createRpcClient(t *testing.T, ctx context.Context) (*grpc.ClientConn, alphabill.AlphabillServiceClient) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	rpcServer, _ := NewGRPCServer(&MockNode{}, observability.Default(t))
	alphabill.RegisterAlphabillServiceServer(grpcServer, rpcServer)
	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Error("gRPC server exited with error:", err)
		}
	}()
	t.Cleanup(func() { grpcServer.Stop() })

	d := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	conn, err := grpc.DialContext(ctx, "", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithContextDialer(d))
	if err != nil {
		t.Fatal(err)
	}
	return conn, alphabill.NewAlphabillServiceClient(conn)
}

func createTransactionOrder(t *testing.T, unitID types.UnitID) []byte {
	bt := &money.TransferAttributes{
		NewBearer:   templates.AlwaysTrueBytes(),
		TargetValue: 1,
		Backlink:    nil,
	}

	attBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)

	order, err := cbor.Marshal(&types.TransactionOrder{
		Payload: &types.Payload{
			UnitID:         unitID,
			Type:           money.PayloadTypeTransfer,
			Attributes:     attBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 0},
		},
		OwnerProof: []byte{1},
	})
	require.NoError(t, err)
	return order
}
