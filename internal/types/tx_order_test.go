package types

import (
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

type Attributes struct {
	_           struct{} `cbor:",toarray"`
	NewBearer   []byte
	TargetValue uint64
	Backlink    []byte
}

var (
	systemID                     = []byte{1, 0, 0, 1}
	payloadAttributesType        = "transfer"
	unitID                       = make([]byte, 32)
	timeout               int64  = 42
	maxFee                int64  = 69
	feeCreditRecordID            = []byte{32, 32, 32, 32}
	newBearer                    = []byte{1, 2, 3, 4}
	targetValue           uint64 = 100
	backlink                     = make([]byte, 32)

	// 85                                       # array(5)
	//   44                                     #   bytes(4)
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
	payloadInHEX = "85" +
		"4401000001" + // SystemID
		"687472616E73666572" + // Type
		"58200000000000000000000000000000000000000000000000000000000000000000" + // UnitID
		"834401020304186458200000000000000000000000000000000000000000000000000000000000000000" + // Attributes
		"83182A18454420202020" // Client metadata
)

func TestMarshalPayload(t *testing.T) {
	payloadBytes, err := createTxOrder(t).PayloadBytes()
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
		SystemID:       nil,
		Type:           "",
		UnitID:         nil,
		Attributes:     nil,
		ClientMetadata: nil,
	}, OwnerProof: make([]byte, 32)}
	payloadBytes, err := order.PayloadBytes()
	fmt.Printf("%X", payloadBytes)
	require.NoError(t, err)
	// 85    # array(5)
	//   f6 #   null, simple(22)
	//   60 #   text(0)
	//      #     ""
	//   f6 #   null, simple(22)
	//   f6 #   null, simple(22)
	//   f6 #   null, simple(22)
	require.Equal(t, []byte{0x85, 0xf6, 0x60, 0xf6, 0xf6, 0xf6}, payloadBytes)
}

func TestUnmarshalPayload(t *testing.T) {
	payload := &Payload{}
	require.NoError(t, cbor.Unmarshal(hexDecode(t, payloadInHEX), payload))

	require.Equal(t, SystemID(systemID), payload.SystemID)
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

func createTxOrder(t *testing.T) *TransactionOrder {
	attributes := &Attributes{NewBearer: newBearer, TargetValue: targetValue, Backlink: backlink}

	attr, err := cbor.Marshal(attributes)
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
