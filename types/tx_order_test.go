package types

import (
	"crypto"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

type Attributes struct {
	_           struct{} `cbor:",toarray"`
	NewBearer   []byte
	TargetValue uint64
	Backlink    []byte
}

var (
	systemID              SystemID = 0x01000001
	payloadAttributesType          = "transfer"
	unitID                         = make([]byte, 32)
	timeout               uint64   = 42
	maxFee                uint64   = 69
	feeCreditRecordID              = []byte{32, 32, 32, 32}
	newBearer                      = []byte{1, 2, 3, 4}
	targetValue           uint64   = 100
	backlink                       = make([]byte, 32)

	// 86                                       # array(6)
	//   1A                                     #   uint32
	//      01000001                            #     "\x01\x00\x00\x01"
	//   68                                     #   text(8)
	//      7472616e73666572                    #     "transfer"
	//   58 20                                  #   bytes(32)
	//      00000000000000000000000000000000    #     "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//      00000000000000000000000000000000    #     "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//   83                                     #   array(3)
	//      44                                  #     bytes(4)
	//         01020304                         #       "\x01\x02\x03\x04"
	//      18 64                               #     unsigned(100)
	//      58 20                               #     bytes(32)
	//         00000000000000000000000000000000 #       "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//         00000000000000000000000000000000 #       "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
	//   83                                     #   array(3)
	//      18 2a                               #     unsigned(42)
	//      18 45                               #     unsigned(69)
	//      44                                  #     bytes(4)
	//         20202020                         #       "    "
	payloadInHEX = "86" +
		"1A01000001" + // SystemID
		"687472616E73666572" + // Type
		"58200000000000000000000000000000000000000000000000000000000000000000" + // UnitID
		"834401020304186458200000000000000000000000000000000000000000000000000000000000000000" + // Attributes
		"f6" + // State lock
		"83182A18454420202020" // Client metadata
)

func TestMarshalPayload(t *testing.T) {
	payloadBytes, err := createTxOrder(t).PayloadBytes()
	fmt.Printf("payloadBytes: %x\n", payloadBytes)
	require.NoError(t, err)
	require.Equal(t, hexDecode(t, payloadInHEX), payloadBytes)
}

func TestMarshalNilPayload(t *testing.T) {
	order := &TransactionOrder{Payload: nil, OwnerProof: make([]byte, 32)}
	payloadBytes, err := order.PayloadBytes()
	require.NoError(t, err)
	require.Equal(t, cborNil, payloadBytes)
}

func TestMarshalNilValuesInPayload(t *testing.T) {
	order := &TransactionOrder{Payload: &Payload{
		SystemID:       0,
		Type:           "",
		UnitID:         nil,
		Attributes:     nil,
		ClientMetadata: nil,
	}, OwnerProof: make([]byte, 32)}
	payloadBytes, err := order.PayloadBytes()
	require.NoError(t, err)
	// 86    # array(6)
	//   00 #   zero, unsigned int
	//   60 #   text(0)
	//      #     ""
	//   f6 #   null, simple(22)
	//   f6 #   null, simple(22)
	//   f6 #   null, simple(22)
	//   f6 #   null, simple(22)
	require.Equal(t, []byte{0x86, 0x00, 0x60, 0xf6, 0xf6, 0xf6, 0xf6}, payloadBytes)

	payload := &Payload{}
	require.NoError(t, Cbor.Unmarshal(payloadBytes, payload))
	require.EqualValues(t, order.Payload, payload)
}

func TestUnmarshalPayload(t *testing.T) {
	payload := &Payload{}
	require.NoError(t, Cbor.Unmarshal(hexDecode(t, payloadInHEX), payload))

	require.Equal(t, systemID, payload.SystemID)
	require.Equal(t, payloadAttributesType, payload.Type)
	require.Equal(t, UnitID(unitID), payload.UnitID)

	attributes := &Attributes{}
	require.NoError(t, payload.UnmarshalAttributes(attributes))
	require.Equal(t, newBearer, attributes.NewBearer)
	require.Equal(t, targetValue, attributes.TargetValue)
	require.Equal(t, backlink, attributes.Backlink)

	clientMetadata := payload.ClientMetadata
	require.NotNil(t, clientMetadata)
	require.Equal(t, timeout, clientMetadata.Timeout)
	require.Equal(t, maxFee, clientMetadata.MaxTransactionFee)
	require.Equal(t, feeCreditRecordID, clientMetadata.FeeCreditRecordID)
}

func TestUnmarshalAttributes(t *testing.T) {
	txOrder := createTxOrder(t)
	attributes := &Attributes{}
	require.NoError(t, txOrder.UnmarshalAttributes(attributes))
	require.Equal(t, newBearer, attributes.NewBearer)
	require.Equal(t, targetValue, attributes.TargetValue)
	require.Equal(t, backlink, attributes.Backlink)
	require.Equal(t, UnitID(unitID), txOrder.UnitID())
	require.Equal(t, systemID, txOrder.SystemID())
	require.Equal(t, timeout, txOrder.Timeout())
	require.Equal(t, payloadAttributesType, txOrder.PayloadType())
	require.Equal(t, feeCreditRecordID, txOrder.GetClientFeeCreditRecordID())
	require.Equal(t, maxFee, txOrder.GetClientMaxTxFee())
	require.NotNil(t, txOrder.Hash(crypto.SHA256))
}

func Test_TransactionOrder_SetOwnerProof(t *testing.T) {
	t.Run("proofer returns error", func(t *testing.T) {
		expErr := errors.New("proofing failed")
		txo := TransactionOrder{}
		err := txo.SetOwnerProof(func(bytesToSign []byte) ([]byte, error) { return nil, expErr })
		require.ErrorIs(t, err, expErr)
	})

	t.Run("success", func(t *testing.T) {
		proof := []byte{1, 2, 3, 4, 5, 6}
		txo := TransactionOrder{}
		err := txo.SetOwnerProof(func(bytesToSign []byte) ([]byte, error) { return proof, nil })
		require.NoError(t, err)
		require.Equal(t, proof, txo.OwnerProof)
	})
}

func Test_Payload_SetAttributes(t *testing.T) {
	attributes := &Attributes{NewBearer: []byte{9, 3, 5, 2, 6}, TargetValue: 59, Backlink: []byte{4, 2, 7, 5}}
	pl := Payload{}
	require.NoError(t, pl.SetAttributes(attributes))

	attrData := &Attributes{}
	require.NoError(t, pl.UnmarshalAttributes(attrData))
	require.Equal(t, attributes, attrData, "expected to get back the same attributes")
}

func createTxOrder(t *testing.T) *TransactionOrder {
	attributes := &Attributes{NewBearer: newBearer, TargetValue: targetValue, Backlink: backlink}

	attr, err := Cbor.Marshal(attributes)
	require.NoError(t, err)

	order := &TransactionOrder{Payload: &Payload{
		SystemID:   systemID,
		Type:       payloadAttributesType,
		UnitID:     unitID,
		Attributes: attr,
		ClientMetadata: &ClientMetadata{
			Timeout:           timeout,
			MaxTransactionFee: maxFee,
			FeeCreditRecordID: feeCreditRecordID,
		},
	}}
	return order
}

func hexDecode(t *testing.T, s string) []byte {
	data, err := hex.DecodeString(s)
	if err != nil {
		require.NoError(t, err)
	}
	return data
}
