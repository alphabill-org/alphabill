package tokens

import (
	"bytes"
	"context"
	"crypto"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/txsubmitter"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestConfirmUnitsTx_skip(t *testing.T) {
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
			return nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	batch.Add(txsubmitter.New(&types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 1}}}))
	err := batch.SendTx(context.Background(), false)
	require.NoError(t, err)

}

func TestConfirmUnitsTx_ok(t *testing.T) {
	getRoundNumberCalled := false
	getTxProofCalled := false
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled = true
			return 100, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
			getTxProofCalled = true
			return &sdk.Proof{}, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	batch.Add(txsubmitter.New(&types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}}}))
	err := batch.SendTx(context.Background(), true)
	require.NoError(t, err)
	require.True(t, getRoundNumberCalled)
	require.True(t, getTxProofCalled)
}

func TestConfirmUnitsTx_timeout(t *testing.T) {
	getRoundNumberCalled := 0
	getTxProofCalled := 0
	randomID1 := test.RandomBytes(32)
	backend := &mockTokenBackend{
		postTransactions: func(ctx context.Context, pubKey sdk.PubKey, txs *sdk.Transactions) error {
			return nil
		},
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			if getRoundNumberCalled == 1 {
				return 100, nil
			}
			return 102, nil
		},
		getTxProof: func(ctx context.Context, unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
			getTxProofCalled++
			if bytes.Equal(unitID, randomID1) {
				return &sdk.Proof{}, nil
			}
			return nil, nil
		},
	}
	batch := txsubmitter.NewBatch(nil, backend)
	sub1 := txsubmitter.New(&types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 101}, UnitID: randomID1}})
	batch.Add(sub1)
	sub2 := txsubmitter.New(&types.TransactionOrder{Payload: &types.Payload{ClientMetadata: &types.ClientMetadata{Timeout: 102}}})
	batch.Add(sub2)
	err := batch.SendTx(context.Background(), true)
	require.ErrorContains(t, err, "confirmation timeout")
	require.EqualValues(t, 2, getRoundNumberCalled)
	require.EqualValues(t, 2, getTxProofCalled)
	require.True(t, sub1.Confirmed())
	require.False(t, sub2.Confirmed())
}

func TestCachingRoundNumberFetcher(t *testing.T) {
	getRoundNumberCalled := 0
	backend := &mockTokenBackend{
		getRoundNumber: func(ctx context.Context) (uint64, error) {
			getRoundNumberCalled++
			return 100, nil
		},
	}
	fetcher := &cachingRoundNumberFetcher{delegate: backend.GetRoundNumber}
	num, err := fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, num)
	require.EqualValues(t, 1, getRoundNumberCalled)
	num, err = fetcher.getRoundNumber(context.Background())
	require.NoError(t, err)
	require.EqualValues(t, 100, num)
	require.EqualValues(t, 1, getRoundNumberCalled)
}
func TestGetTxFungibleAmount_TransferFungibleToken(t *testing.T) {
	attributes, _ := cbor.Marshal(ttxs.TransferFungibleTokenAttributes{
		Value: 100,
	})
	tx := &sdk.TransactionOrder{
		Payload: &types.Payload{
			Type:       ttxs.PayloadTypeTransferFungibleToken,
			Attributes: attributes,
		},
	}
	unitID, value, err := GetTxFungibleAmount(tx)
	require.NoError(t, err)
	require.Equal(t, tx.UnitID(), unitID)
	require.Equal(t, uint64(100), value)
}

func TestGetTxFungibleAmount_SplitFungibleToken(t *testing.T) {
	attributes, _ := cbor.Marshal(ttxs.SplitFungibleTokenAttributes{
		TargetValue: 50,
	})
	tx := &sdk.TransactionOrder{
		Payload: &types.Payload{
			Type:       ttxs.PayloadTypeSplitFungibleToken,
			Attributes: attributes,
		},
	}
	unitID, value, err := GetTxFungibleAmount(tx)
	require.NoError(t, err)
	require.EqualValues(t, ttxs.NewFungibleTokenID(tx.UnitID(), ttxs.HashForIDCalculation(tx.Cast(), crypto.SHA256)), unitID)
	require.Equal(t, uint64(50), value)
}

func TestGetTxFungibleAmount_BurnFungibleToken(t *testing.T) {
	attributes, _ := cbor.Marshal(ttxs.BurnFungibleTokenAttributes{
		Value: 100,
	})
	tx := &sdk.TransactionOrder{
		Payload: &types.Payload{
			Type:       ttxs.PayloadTypeBurnFungibleToken,
			Attributes: attributes,
		},
	}
	unitID, value, err := GetTxFungibleAmount(tx)
	require.NoError(t, err)
	require.Equal(t, tx.UnitID(), unitID)
	require.Equal(t, uint64(100), value)
}

func TestGetTxFungibleAmount_UnsupportedTxType(t *testing.T) {
	tx := &sdk.TransactionOrder{
		Payload: &types.Payload{
			Type: "unsupported",
		},
	}
	_, _, err := GetTxFungibleAmount(tx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported tx type")
}
