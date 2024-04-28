package rpc

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"hash"
	"io"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/money"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/network"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
)

var unitID = types.NewUnitID(33, nil, []byte{5}, []byte{0xFF})

func TestGetRoundNumber(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node, nil)

	t.Run("ok", func(t *testing.T) {
		node.maxRoundNumber = 1337

		roundNumber, err := api.GetRoundNumber(context.Background())
		require.NoError(t, err)
		require.EqualValues(t, 1337, roundNumber)
	})
	t.Run("err", func(t *testing.T) {
		node.err = errors.New("some error")
		node.maxRoundNumber = 0

		roundNumber, err := api.GetRoundNumber(context.Background())
		require.ErrorContains(t, err, "some error")
		require.EqualValues(t, 0, roundNumber)
	})
}

func TestGetUnit(t *testing.T) {
	node := &MockNode{
		txs: &testtxsystem.CounterTxSystem{
			FixedState: prepareState(t),
		},
	}
	api := NewStateAPI(node, nil)

	t.Run("get unit (proof=false)", func(t *testing.T) {
		unit, err := api.GetUnit(unitID, false)
		require.NoError(t, err)
		require.NotNil(t, unit)

		require.NotNil(t, unit)
		require.NotNil(t, unit.Data)
		require.Nil(t, unit.StateProof)
		require.EqualValues(t, types.Bytes{0x83, 0x00, 0x41, 0x01, 0xF6}, unit.OwnerPredicate)
	})
	t.Run("get unit (proof=true)", func(t *testing.T) {
		unit, err := api.GetUnit(unitID, true)
		require.NoError(t, err)
		require.NotNil(t, unit)
		require.NotNil(t, unit.Data)
		require.NotNil(t, unit.StateProof)
		require.EqualValues(t, unitID, unit.StateProof.UnitID)
	})
	t.Run("unit not found", func(t *testing.T) {
		unit, err := api.GetUnit([]byte{1, 2, 3}, false)
		require.NoError(t, err)
		require.Nil(t, unit)
	})
}

func TestGetUnitsByOwnerID(t *testing.T) {
	node := &MockNode{}
	ownerIndex := &MockOwnerIndex{ownerUnits: map[string][]types.UnitID{}}
	api := NewStateAPI(node, ownerIndex)

	t.Run("ok", func(t *testing.T) {
		ownerID := []byte{1}
		ownerIndex.ownerUnits[string(ownerID)] = []types.UnitID{[]byte{0}, []byte{1}}

		unitIds, err := api.GetUnitsByOwnerID(ownerID)
		require.NoError(t, err)
		require.Len(t, unitIds, 2)
		require.EqualValues(t, []byte{0}, unitIds[0])
		require.EqualValues(t, []byte{1}, unitIds[1])
	})
	t.Run("err", func(t *testing.T) {
		ownerID := []byte{1}
		ownerIndex.err = errors.New("some error")

		unitIds, err := api.GetUnitsByOwnerID(ownerID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, unitIds)
	})
}

func TestSendTransaction(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node, nil)

	t.Run("ok", func(t *testing.T) {
		tx := createTransactionOrder(t, []byte{1})
		txHash, err := api.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
		require.NotNil(t, txHash)
	})
	t.Run("err", func(t *testing.T) {
		tx := createTransactionOrder(t, failingUnitID)
		txHash, err := api.SendTransaction(context.Background(), tx)
		require.ErrorContains(t, err, "failed")
		require.Nil(t, txHash)
	})
}

func TestGetTransactionProof(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node, nil)

	t.Run("ok", func(t *testing.T) {
		txHash := []byte{1}
		res, err := api.GetTransactionProof(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.TxRecord)
		require.NotNil(t, res.TxProof)

		var txRecord *types.TransactionRecord
		err = types.Cbor.Unmarshal(res.TxRecord, &txRecord)
		require.NoError(t, err)

		var txProof *types.TxProof
		err = types.Cbor.Unmarshal(res.TxProof, &txProof)
		require.NoError(t, err)
	})
	t.Run("err", func(t *testing.T) {
		txHash := []byte{1}
		node.err = errors.New("some error")

		res, err := api.GetTransactionProof(context.Background(), txHash)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, res)
	})
}

func TestGetBlock(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node, nil)

	t.Run("ok", func(t *testing.T) {
		node.maxBlockNumber = 1
		blockNumber := types.Uint64(1)
		res, err := api.GetBlock(context.Background(), blockNumber)
		require.NoError(t, err)
		require.NotNil(t, res)

		var block *types.Block
		err = types.Cbor.Unmarshal(res, &block)
		require.NoError(t, err)
		require.EqualValues(t, 1, block.GetRoundNumber())
	})
	t.Run("block not found", func(t *testing.T) {
		node.maxBlockNumber = 1
		blockNumber := types.Uint64(2)

		res, err := api.GetBlock(context.Background(), blockNumber)
		require.NoError(t, err)
		require.Nil(t, res)
	})
}

func prepareState(t *testing.T) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		state.AddUnit(unitID, templates.AlwaysTrueBytes(), &unitData{I: 10}),
	))

	require.NoError(t, s.AddUnitLog(unitID, test.RandomBytes(32)))

	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	return s
}

type unitData struct {
	_ struct{} `cbor:",toarray"`
	I uint64
}

func (ud *unitData) Hash(hashAlgo crypto.Hash) []byte {
	hasher := hashAlgo.New()
	_ = ud.Write(hasher)
	return hasher.Sum(nil)
}

func (ud *unitData) Write(hasher hash.Hash) error {
	res, err := types.Cbor.Marshal(ud)
	if err != nil {
		return fmt.Errorf("unit data encode error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (ud *unitData) SummaryValueInput() uint64 {
	return ud.I
}

func (ud *unitData) Copy() types.UnitData {
	return &unitData{I: ud.I}
}

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

func (mn *MockNode) SystemID() types.SystemID {
	return 0x00010000
}

func (mn *MockNode) Peer() *network.Peer {
	return nil
}

func (mn *MockNode) ValidatorNodes() peer.IDSlice {
	return []peer.ID{}
}

func (mn *MockNode) SerializeState(writer io.Writer) error {
	if mn.err != nil {
		return mn.err
	}
	return nil
}

func (mn *MockOwnerIndex) GetOwnerUnits(ownerID []byte) ([]types.UnitID, error) {
	if mn.err != nil {
		return nil, mn.err
	}
	return mn.ownerUnits[string(ownerID)], nil
}

func createTransactionOrder(t *testing.T, unitID types.UnitID) []byte {
	bt := &money.TransferAttributes{
		NewBearer:   templates.AlwaysTrueBytes(),
		TargetValue: 1,
		Counter:     0,
	}

	attBytes, err := types.Cbor.Marshal(bt)
	require.NoError(t, err)

	order, err := types.Cbor.Marshal(&types.TransactionOrder{
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
