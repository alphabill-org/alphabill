package wvm

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/stretchr/testify/require"
)

func Test_ABTypesFactory_createObj(t *testing.T) {
	t.Run("unknown type id", func(t *testing.T) {
		f := ABTypesFactory{}
		obj, err := f.createObj(0, nil)
		require.EqualError(t, err, `unknown type ID 0`)
		require.Empty(t, obj)
	})

	t.Run("valid type id but empty input", func(t *testing.T) {
		f := ABTypesFactory{}
		obj, err := f.createObj(type_id_tx_order, nil)
		require.EqualError(t, err, `decoding data as *types.TransactionOrder: EOF`)
		require.Empty(t, obj)

		obj, err = f.createObj(type_id_tx_order, []byte{})
		require.EqualError(t, err, `decoding data as *types.TransactionOrder: EOF`)
		require.Empty(t, obj)
	})

	t.Run("invalid data", func(t *testing.T) {
		f := ABTypesFactory{}
		obj, err := f.createObj(type_id_tx_order, []byte{99, 98, 12, 33, 44, 10})
		require.EqualError(t, err, `decoding data as *types.TransactionOrder: cbor: 2 bytes of extraneous data starting at index 4`)
		require.Empty(t, obj)
	})

	t.Run("TransactionOrder ok", func(t *testing.T) {
		txo := types.TransactionOrder{
			Payload: types.Payload{
				SystemID: 1,
				Type:     22,
			},
			AuthProof: []byte{1},
			FeeProof:  []byte{2},
		}
		buf, err := types.Cbor.Marshal(txo)
		require.NoError(t, err)

		f := ABTypesFactory{}
		obj, err := f.createObj(type_id_tx_order, buf)
		require.NoError(t, err)
		require.Equal(t, &txo, obj)
	})

	t.Run("TransactionRecord ok", func(t *testing.T) {
		tx := &types.TransactionOrder{}
		require.NoError(t, tx.SetAuthProof([]byte{0, 0, 0}))
		txBytes, err := tx.MarshalCBOR()
		require.NoError(t, err)
		txr := types.TransactionRecord{Version: 1,
			TransactionOrder: txBytes,
			ServerMetadata: &types.ServerMetadata{
				ActualFee:        24,
				SuccessIndicator: types.TxStatusSuccessful,
			},
		}
		buf, err := types.Cbor.Marshal(txr)
		require.NoError(t, err)

		f := ABTypesFactory{}
		obj, err := f.createObj(type_id_tx_record, buf)
		require.NoError(t, err)
		require.Equal(t, &txr, obj)
	})

	t.Run("TxProof ok", func(t *testing.T) {
		txp := types.TxProof{Version: 1,
			BlockHeaderHash: []byte{5, 5, 5},
			Chain:           []*types.GenericChainItem{{Hash: []byte{4}}},
		}
		buf, err := types.Cbor.Marshal(txp)
		require.NoError(t, err)

		f := ABTypesFactory{}
		obj, err := f.createObj(type_id_tx_proof, buf)
		require.NoError(t, err)
		require.Equal(t, &txp, obj)
	})
}
