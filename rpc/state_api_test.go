package rpc

import (
	"context"
	"crypto"
	"errors"
	"fmt"
	"hash"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var unitID = types.NewUnitID(33, nil, []byte{5}, []byte{0xFF})

func TestGetRoundNumber(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node)

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
	api := NewStateAPI(node)

	t.Run("get unit (proof=false)", func(t *testing.T) {
		unit, err := api.GetUnit(unitID, false)
		require.NoError(t, err)
		require.NotNil(t, unit)

		require.NotNil(t, unit)
		require.NotNil(t, unit.Data)
		require.Nil(t, unit.StateProof)
		require.EqualValues(t, types.PredicateBytes{0x83, 0x00, 0x01, 0xF6}, unit.OwnerPredicate)
	})
	t.Run("get unit (proof=true)", func(t *testing.T) {
		unit, err := api.GetUnit(unitID, true)
		require.NoError(t, err)
		require.NotNil(t, unit)
		require.NotNil(t, unit.Data)
		require.NotNil(t, unit.StateProof)
		require.EqualValues(t, unitID, unit.StateProof.UnitID)
	})
	t.Run("err", func(t *testing.T) {
		unitData, err := api.GetUnit([]byte{1, 2, 3}, false)
		require.ErrorContains(t, err, "item 010203 does not exist")
		require.Nil(t, unitData)
	})
}

func TestGetUnitsByOwnerID(t *testing.T) {
	node := &MockNode{ownerUnits: map[string][]types.UnitID{}}
	api := NewStateAPI(node)

	t.Run("ok", func(t *testing.T) {
		ownerID := []byte{1}
		node.ownerUnits[string(ownerID)] = []types.UnitID{[]byte{0}, []byte{1}}

		unitIds, err := api.GetUnitsByOwnerID(ownerID)
		require.NoError(t, err)
		require.Len(t, unitIds, 2)
		require.EqualValues(t, []byte{0}, unitIds[0])
		require.EqualValues(t, []byte{1}, unitIds[1])
	})
	t.Run("err", func(t *testing.T) {
		ownerID := []byte{1}
		node.err = errors.New("some error")

		unitIds, err := api.GetUnitsByOwnerID(ownerID)
		require.ErrorContains(t, err, "some error")
		require.Nil(t, unitIds)
	})
}

func TestSendTransaction(t *testing.T) {
	node := &MockNode{}
	api := NewStateAPI(node)

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
	api := NewStateAPI(node)

	t.Run("ok", func(t *testing.T) {
		txHash := []byte{1}
		res, err := api.GetTransactionProof(context.Background(), txHash)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.TxRecord)
		require.NotNil(t, res.TxProof)

		var txRecord *types.TransactionRecord
		err = cbor.Unmarshal(res.TxRecord, &txRecord)
		require.NoError(t, err)

		var txProof *types.TxProof
		err = cbor.Unmarshal(res.TxProof, &txProof)
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
	api := NewStateAPI(node)

	t.Run("ok", func(t *testing.T) {
		node.maxBlockNumber = 1
		blockNumber := uint64(1)
		res, err := api.GetBlock(context.Background(), blockNumber)
		require.NoError(t, err)
		require.NotNil(t, res)

		var block *types.Block
		err = cbor.Unmarshal(res, &block)
		require.NoError(t, err)
		require.EqualValues(t, 1, block.GetRoundNumber())
	})
	t.Run("err", func(t *testing.T) {
		blockNumber := uint64(1)
		node.err = errors.New("some error")

		res, err := api.GetBlock(context.Background(), blockNumber)
		require.ErrorContains(t, err, "some error")
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
	enc, err := cbor.CanonicalEncOptions().EncMode()
	if err != nil {
		return err
	}
	res, err := enc.Marshal(ud)
	if err != nil {
		return fmt.Errorf("unit data encode error: %w", err)
	}
	_, err = hasher.Write(res)
	return err
}

func (ud *unitData) SummaryValueInput() uint64 {
	return ud.I
}

func (ud *unitData) Copy() state.UnitData {
	return &unitData{I: ud.I}
}
