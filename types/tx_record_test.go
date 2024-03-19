package types

import (
	"crypto"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/stretchr/testify/require"
)

func TestTransactionRecordFunctions(t *testing.T) {
	txOrder := createTxOrder(t)
	payloadBytes, _ := txOrder.PayloadBytes()
	serverMetadata := &ServerMetadata{
		ActualFee:         1,
		TargetUnits:       []UnitID{txOrder.UnitID()},
		SuccessIndicator:  TxStatusSuccessful,
		ProcessingDetails: payloadBytes,
	}
	transactionRecord := &TransactionRecord{
		TransactionOrder: txOrder,
		ServerMetadata:   serverMetadata,
	}
	expectedHash := "0x25ab5dbcca42fe18dcec00d91d66f8b3b8dc7fe1b3a618e52d96b654983ad21f"
	expectedBytes, _ := cbor.Marshal(transactionRecord)

	t.Run("Test Hash", func(t *testing.T) {
		require.Equal(t, expectedHash, hexutil.Encode(transactionRecord.Hash(crypto.SHA256)))
	})

	t.Run("Test Bytes", func(t *testing.T) {
		bytes, err := transactionRecord.Bytes()
		require.NoError(t, err)
		require.Equal(t, expectedBytes, bytes)
	})

	t.Run("Test UnmarshalProcessingDetails", func(t *testing.T) {
		var payload Payload
		err := transactionRecord.UnmarshalProcessingDetails(&payload)
		require.NoError(t, err)
		require.Equal(t, txOrder.Payload.UnitID, payload.UnitID)
		require.Equal(t, txOrder.Payload.SystemID, payload.SystemID)
		require.Equal(t, txOrder.Payload.Type, payload.Type)
		require.Equal(t, txOrder.Payload.Attributes, payload.Attributes)
		require.Equal(t, txOrder.Payload.ClientMetadata, payload.ClientMetadata)
	})

	t.Run("Test GetActualFee", func(t *testing.T) {
		require.EqualValues(t, uint64(1), transactionRecord.GetActualFee())
		require.EqualValues(t, uint64(1), serverMetadata.GetActualFee())
	})

	t.Run("Test UnmarshalDetails", func(t *testing.T) {
		var payload Payload
		err := serverMetadata.UnmarshalDetails(&payload)
		require.NoError(t, err)
		require.Equal(t, txOrder.Payload.UnitID, payload.UnitID)
		require.Equal(t, txOrder.Payload.SystemID, payload.SystemID)
		require.Equal(t, txOrder.Payload.Type, payload.Type)
		require.Equal(t, txOrder.Payload.Attributes, payload.Attributes)
		require.Equal(t, txOrder.Payload.ClientMetadata, payload.ClientMetadata)
	})
}
