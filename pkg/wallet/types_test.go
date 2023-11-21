package wallet

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestTransactionOrderUnmarshalling(t *testing.T) {
	txo := &types.TransactionOrder{}

	var tx = TransactionOrder(*txo)

	tx = TransactionOrder{Payload: &types.Payload{Type: "testTx"}}

	txBytes, err := cbor.Marshal(tx)
	require.NoError(t, err)

	err = cbor.Unmarshal(txBytes, txo)
	require.NoError(t, err)

	require.Equal(t, tx.Payload.Type, txo.Payload.Type)
}
