package statedb

import (
	"crypto"
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/holiman/uint256"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/stretchr/testify/require"
)

func TestStateObject_Write(t *testing.T) {
	key1 := common.BigToHash(big.NewInt(1))
	value1 := common.BigToHash(big.NewInt(2))
	key2 := common.BigToHash(big.NewInt(3))
	value2 := common.BigToHash(big.NewInt(4))

	so := &StateObject{
		Address: common.BytesToAddress(test.RandomBytes(20)),
		Account: &Account{
			Balance:  uint256.NewInt(200),
			CodeHash: emptyCodeHash,
			Nonce:    2,
		},
		Storage: state.Storage{key2: value2, key1: value1},
		AlphaBill: &AlphaBillLink{
			TxHash:  test.RandomBytes(32),
			Timeout: 10,
		},
		Suicided: false,
	}
	hasher := crypto.SHA256.New()
	res, err := types.Cbor.Marshal(so)
	require.NoError(t, err)
	hasher.Write(res)
	expectedHash := hasher.Sum(nil)
	hasher.Reset()
	require.NoError(t, so.Write(hasher))
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
	// make sure all fields where serialized
	var soFormSerialized StateObject
	require.NoError(t, types.Cbor.Unmarshal(res, &soFormSerialized))
	require.Equal(t, so, &soFormSerialized)
}

func TestAlphaBillLink_GetTimeout(t *testing.T) {
	t.Run("nil case", func(t *testing.T) {
		var abLink *AlphaBillLink = nil
		require.EqualValues(t, 0, abLink.GetTimeout())
	})
	t.Run("timeout set", func(t *testing.T) {
		abLink := &AlphaBillLink{
			TxHash:  test.RandomBytes(32),
			Timeout: 10,
		}
		require.EqualValues(t, 10, abLink.GetTimeout())
	})
}
