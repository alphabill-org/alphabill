package main

import (
	"context"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/rpc/alphabill"
	fct "github.com/alphabill-org/alphabill/txsystem/fc/types"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

var (
	pubKey         = test.RandomBytes(33)
	unitID         = test.RandomBytes(33)
	targetUnitID   = test.RandomBytes(33)
	fcrID          = test.RandomBytes(33)
	backlink       = test.RandomBytes(33)
	ownerCondition = test.RandomBytes(33)
	maxFee         = fct.Fee(10)
	billValue      = uint64(100)
	timeout        = uint64(200)
)

type MockAlphabillServiceClient struct {
	alphabill.AlphabillServiceClient
}

func (m *MockAlphabillServiceClient) GetRoundNumber(ctx context.Context, in *emptypb.Empty, opts ...grpc.CallOption) (*alphabill.GetRoundNumberResponse, error) {
	return &alphabill.GetRoundNumberResponse{
		RoundNumber: timeout,
	}, nil
}

func (m *MockAlphabillServiceClient) ProcessTransaction(ctx context.Context, in *alphabill.Transaction, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return nil, nil
}

func (m *MockAlphabillServiceClient) GetBlock(ctx context.Context, in *alphabill.GetBlockRequest, opts ...grpc.CallOption) (*alphabill.GetBlockResponse, error) {
	tx, err := createTransferTx(pubKey, unitID, billValue, fcrID, timeout, backlink)
	b := &types.Block{
		Header: &types.Header{
			SystemID:          money.DefaultSystemIdentifier,
			ShardID:           test.RandomBytes(33),
			ProposerID:        "proposer123",
			PreviousBlockHash: test.RandomBytes(33),
		},
		Transactions:       []*types.TransactionRecord{{TransactionOrder: tx}},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}
	bytes, err := cbor.Marshal(b)
	if err != nil {
		return nil, err
	}
	return &alphabill.GetBlockResponse{
		Block: bytes,
	}, nil
}

func TestCreateTransferFC(t *testing.T) {
	tx, err := createTransferFC(maxFee, unitID, targetUnitID, 100, 200)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestCreateAddFC(t *testing.T) {
	tx, err := createAddFC(unitID, ownerCondition, &types.TransactionRecord{}, &types.TxProof{}, timeout, maxFee)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestCreateTransferTx(t *testing.T) {
	tx, err := createTransferTx(pubKey, unitID, billValue, fcrID, timeout, backlink)

	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, unitID, tx.Payload.UnitID)
}

func TestWaitForConfirmation(t *testing.T) {
	mockClient := new(MockAlphabillServiceClient)

	_, err := waitForConfirmation(context.Background(), mockClient, &types.TransactionOrder{Payload: &types.Payload{UnitID: unitID}}, 100, 200)

	require.NoError(t, err)
}

func TestWaitForConfirmation_Error(t *testing.T) {
	mockClient := new(MockAlphabillServiceClient)

	_, err := waitForConfirmation(context.Background(), mockClient, &types.TransactionOrder{}, 100, 200)

	require.ErrorContains(t, err, "tx failed to confirm")
}

func TestExecBill_Error(t *testing.T) {
	mockClient := new(MockAlphabillServiceClient)

	require.ErrorContains(t, execInitialBill(context.Background(), mockClient, 10, unitID, billValue, ownerCondition), "tx failed to confirm")

}
