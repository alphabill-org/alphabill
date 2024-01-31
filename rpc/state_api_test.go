package rpc

import (
	"context"
	"errors"
	"testing"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

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
	node := &MockNode{}
	api := NewStateAPI(node)

	t.Run("get unit (proof=false, data=false)", func(t *testing.T) {
		unitID := []byte{1}

		unitData, err := api.GetUnit(unitID, false, false)
		require.NoError(t, err)
		require.NotNil(t, unitData)

		var res *types.UnitDataAndProof
		err = cbor.Unmarshal(unitData.DataAndProofCBOR, &res)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Nil(t, res.UnitData)
		require.Nil(t, res.Proof)
	})
	t.Run("get unit (proof=true, data=false)", func(t *testing.T) {
		unitID := []byte{1}

		unitData, err := api.GetUnit(unitID, true, false)
		require.NoError(t, err)
		require.NotNil(t, unitData)

		var res *types.UnitDataAndProof
		err = cbor.Unmarshal(unitData.DataAndProofCBOR, &res)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.Nil(t, res.UnitData)
		require.NotNil(t, res.Proof)
		require.EqualValues(t, unitID, res.Proof.UnitID)
	})
	t.Run("get unit (proof=true, data=true)", func(t *testing.T) {
		unitID := []byte{1}

		unitData, err := api.GetUnit(unitID, true, true)
		require.NoError(t, err)
		require.NotNil(t, unitData)

		var res *types.UnitDataAndProof
		err = cbor.Unmarshal(unitData.DataAndProofCBOR, &res)
		require.NoError(t, err)
		require.NotNil(t, res)
		require.NotNil(t, res.Proof)
		require.EqualValues(t, unitID, res.Proof.UnitID)
		require.NotNil(t, res.UnitData)
		require.EqualValues(t, predicates.PredicateBytes{0x83, 0x00, 0x01, 0xF6}, res.UnitData.Bearer)
	})
	t.Run("err", func(t *testing.T) {
		unitID := []byte{1}
		node.err = errors.New("some error")

		unitData, err := api.GetUnit(unitID, false, false)
		require.ErrorContains(t, err, "some error")
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
		tx := &Transaction{TxOrderCbor: createTransactionOrder(t, []byte{1})}
		txHash, err := api.SendTransaction(context.Background(), tx)
		require.NoError(t, err)
		require.NotNil(t, txHash)
	})
	t.Run("err", func(t *testing.T) {
		tx := &Transaction{TxOrderCbor: createTransactionOrder(t, failingUnitID)}
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
		require.NotNil(t, res.TxRecordCbor)
		require.NotNil(t, res.TxProofCbor)

		var txRecord *types.TransactionRecord
		err = cbor.Unmarshal(res.TxRecordCbor, &txRecord)
		require.NoError(t, err)

		var txProof *types.TxProof
		err = cbor.Unmarshal(res.TxProofCbor, &txProof)
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
		require.NotNil(t, res.BlockCbor)

		var block *types.Block
		err = cbor.Unmarshal(res.BlockCbor, &block)
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
