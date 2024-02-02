package types

import (
	"crypto"
	"testing"

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

	t.Run("Test Hash", func(t *testing.T) {
		require.NotNil(t, transactionRecord.Hash(crypto.SHA256))
	})

	t.Run("Test Bytes", func(t *testing.T) {
		bytes, err := transactionRecord.Bytes()
		require.NoError(t, err)
		require.NotNil(t, bytes)
	})

	t.Run("Test UnmarshalProcessingDetails", func(t *testing.T) {
		var payload Payload
		err := transactionRecord.UnmarshalProcessingDetails(&payload)
		require.NoError(t, err)
	})

	t.Run("Test GetActualFee", func(t *testing.T) {
		require.Equal(t, uint64(1), transactionRecord.GetActualFee())
		require.Equal(t, uint64(1), serverMetadata.GetActualFee())
	})

	t.Run("Test UnmarshalDetails", func(t *testing.T) {
		var payload Payload
		err := serverMetadata.UnmarshalDetails(&payload)
		require.NoError(t, err)
	})
}
