package tokenenc

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

func Test_AuthProof(t *testing.T) {
	t.Run("not CBOR", func(t *testing.T) {
		enc, err := encoder.New(RegisterAuthProof)
		require.NoError(t, err)
		require.NotNil(t, enc)

		txo := types.TransactionOrder{
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeTransferNFT,
			},
			AuthProof: []byte{5, 6, 8, 0, 1, 2, 3},
		}

		b, err := enc.AuthProof(&txo)
		require.EqualError(t, err, `unmarshaling auth proof attributes of tx type 6: cbor: 6 bytes of extraneous data starting at index 1`)
		require.Nil(t, b)
	})

	t.Run("incorrect CBOR", func(t *testing.T) {
		enc, err := encoder.New(RegisterAuthProof)
		require.NoError(t, err)
		require.NotNil(t, enc)

		txo := types.TransactionOrder{
			Payload: types.Payload{
				PartitionID: tokens.DefaultPartitionID,
				Type:        tokens.TransactionTypeUpdateNFT,
			},
		}
		require.NoError(t, txo.SetAuthProof([]byte{1, 2, 3}))

		b, err := enc.AuthProof(&txo)
		require.EqualError(t, err, `unmarshaling auth proof attributes of tx type 12: cbor: cannot unmarshal byte string into Go value of type tokens.UpdateNonFungibleTokenAuthProof`)
		require.Nil(t, b)
	})

	t.Run("success", func(t *testing.T) {
		op := test.RandomBytes(32)

		testCases := []struct {
			TxType    uint16
			AuthProof any
		}{
			{
				TxType:    tokens.TransactionTypeTransferNFT,
				AuthProof: tokens.TransferNonFungibleTokenAuthProof{OwnerProof: op},
			},
			{
				TxType:    tokens.TransactionTypeUpdateNFT,
				AuthProof: tokens.UpdateNonFungibleTokenAuthProof{TokenDataUpdateProof: op},
			},
			{
				TxType:    tokens.TransactionTypeBurnFT,
				AuthProof: tokens.BurnFungibleTokenAuthProof{OwnerProof: op},
			},
			{
				TxType:    tokens.TransactionTypeJoinFT,
				AuthProof: tokens.JoinFungibleTokenAuthProof{OwnerProof: op},
			},
			{
				TxType:    tokens.TransactionTypeSplitFT,
				AuthProof: tokens.SplitFungibleTokenAuthProof{OwnerProof: op},
			},
			{
				TxType:    tokens.TransactionTypeTransferFT,
				AuthProof: tokens.TransferFungibleTokenAuthProof{OwnerProof: op},
			},
		}

		enc, err := encoder.New(RegisterAuthProof)
		require.NoError(t, err)
		require.NotNil(t, enc)

		for x, tc := range testCases {
			txo := types.TransactionOrder{Payload: types.Payload{PartitionID: tokens.DefaultPartitionID, Type: tc.TxType}}
			require.NoError(t, txo.SetAuthProof(tc.AuthProof))

			b, err := enc.AuthProof(&txo)
			require.NoError(t, err, "test case %d", x)
			require.Equal(t, op, b, "test case %d", x)
		}
	})
}
