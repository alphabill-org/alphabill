package atomicity

//
//import (
//	"testing"
//
//	"github.com/alphabill-org/alphabill/internal/types"
//	"github.com/fxamacker/cbor/v2"
//	"github.com/stretchr/testify/require"
//)
//
//type PhiAto struct {
//	_              struct{} `cbor:",toarray"`
//	AtomicityID    types.SystemID
//	UnitID         types.UnitID // atomicity unit
//	BeginRound     uint64
//	EndRound       uint64
//	TargetBearer   []byte // phi'
//	FallbackBearer []byte // phi
//}
//
//type PhiAtoSignature struct {
//	_           struct{} `cbor:",toarray"`
//	AtoTx       []byte
//	ProofKind   ProofKind
//	AtoProof    types.RawCBOR
//	BearerProof []byte
//}
//
//type ProofKind byte
//
//const (
//	TxProof ProofKind = iota
//	UnitProof
//)
//
//type BearerPayload struct {
//	_      struct{} `cbor:",toarray"`
//	Tag    byte
//	ID     uint64
//	Params cbor.RawMessage
//}
//
//func Test_toArray_UnmarshalFirstToStruct(t *testing.T) {
//	type BearerPayloadHeader struct {
//		_   struct{} `cbor:",toarray"`
//		Tag byte
//		ID  uint64
//	}
//
//	type P2pkhPayload struct {
//		_ struct{} `cbor:",toarray"`
//		*BearerPayloadHeader
//		Hash []byte
//	}
//
//	payload := &P2pkhPayload{Hash: []byte{0x02}, BearerPayloadHeader: &BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // [171, 123, h'02']
//	header := &BearerPayloadHeader{}
//	_, err = cbor.UnmarshalFirst(bytes, header)
//	require.NoError(t, err)
//	// fails: cbor: cannot unmarshal array into Go value of type atomicity.BearerPayloadHeader (cannot decode CBOR array to struct with different number of elements)
//	t.Logf("%+v", header)
//}
//
//func Test_toMap_UnmarshalFirstToStruct(t *testing.T) {
//	type BearerPayloadHeader struct {
//		Tag byte
//		ID  uint64
//	}
//
//	type P2pkhPayload struct {
//		*BearerPayloadHeader
//		Hash []byte
//	}
//
//	payload := &P2pkhPayload{Hash: []byte{0x02}, BearerPayloadHeader: &BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // {"Tag": 171, "ID": 123, "Hash": h'02'}
//	sf, _, err := cbor.DiagnoseFirst(bytes)
//	require.NoError(t, err)
//	t.Log(sf) // &{Tag:171 ID:123}
//	header := &BearerPayloadHeader{}
//	_, err = cbor.UnmarshalFirst(bytes, header)
//	require.NoError(t, err)
//	t.Logf("%+v", header) // &{Tag:171 ID:123}
//}
//
//func Test_toArray_UnmarshalFirst_byte(t *testing.T) {
//	type BearerPayloadHeader struct {
//		_   struct{} `cbor:",toarray"`
//		Tag byte
//		ID  uint64
//	}
//
//	type P2pkhPayload struct {
//		_ struct{} `cbor:",toarray"`
//		*BearerPayloadHeader
//		Hash []byte
//	}
//
//	payload := &P2pkhPayload{Hash: []byte{0x02}, BearerPayloadHeader: &BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // [171, 123, h'02']
//	sf, _, err := cbor.DiagnoseFirst(bytes)
//	require.NoError(t, err)
//	t.Log(sf) // [171, 123, h'02']
//	var tag byte
//	_, err = cbor.UnmarshalFirst(bytes, &tag)
//	require.NoError(t, err)
//	// fails: cbor: cannot unmarshal array into Go value of type uint8
//}
//
//func Test_toMap_UnmarshalFirst_byte(t *testing.T) {
//	type BearerPayloadHeader struct {
//		Tag byte
//		ID  uint64
//	}
//	type P2pkhPayload struct {
//		*BearerPayloadHeader
//		Hash []byte
//	}
//	payload := &P2pkhPayload{Hash: []byte{0x02}, BearerPayloadHeader: &BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	require.NoError(t, cbor.Wellformed(bytes))
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // {"Tag": 171, "ID": 123, "Hash": h'02'}
//	sf, _, err := cbor.DiagnoseFirst(bytes)
//	require.NoError(t, err)
//	t.Log(sf) // {"Tag": 171, "ID": 123, "Hash": h'02'}
//	var tag byte
//	_, err = cbor.UnmarshalFirst(bytes, &tag)
//	require.NoError(t, err)
//	// fails: cbor: cannot unmarshal map into Go value of type uint8
//}
//
//func Test_withField_toArray_UnmarshalFirst_byte(t *testing.T) {
//	type BearerPayloadHeader struct {
//		_   struct{} `cbor:",toarray"`
//		Tag byte
//		ID  uint64
//	}
//	type P2pkhPayload struct {
//		_      struct{} `cbor:",toarray"`
//		Header BearerPayloadHeader
//		Hash   []byte
//	}
//	payload := &P2pkhPayload{Hash: []byte{0x02}, Header: *&BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	require.NoError(t, cbor.Wellformed(bytes))
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // [[171, 123], h'02']
//	sf, _, err := cbor.DiagnoseFirst(bytes)
//	require.NoError(t, err)
//	t.Log(sf) // [[171, 123], h'02']
//	var tag byte
//	_, err = cbor.UnmarshalFirst(bytes, &tag)
//	require.NoError(t, err)
//	// fails: cbor: cannot unmarshal map into Go value of type uint8
//}
//
//func Test_withField_noTags_UnmarshalFirst_struct(t *testing.T) {
//	type BearerPayloadHeader struct {
//		Tag byte
//		ID  uint64
//	}
//	type P2pkhPayload struct {
//		Header *BearerPayloadHeader
//		Hash   []byte
//	}
//	payload := &P2pkhPayload{Hash: []byte{0x02}, Header: &BearerPayloadHeader{Tag: 0xAB, ID: 123}}
//	bytes, err := cbor.Marshal(payload)
//	require.NoError(t, err)
//	t.Logf("%X", bytes)
//	//cbor.NewEncoder(nil).
//	require.NoError(t, cbor.Wellformed(bytes))
//	s, err := cbor.Diagnose(bytes)
//	require.NoError(t, err)
//	t.Log(s) // {"Header": {"Tag": 171, "ID": 123}, "Hash": h'02'}
//	sf, _, err := cbor.DiagnoseFirst(bytes)
//	require.NoError(t, err)
//	t.Log(sf) // {"Header": {"Tag": 171, "ID": 123}, "Hash": h'02'}
//	header := &BearerPayloadHeader{}
//	_, err = cbor.UnmarshalFirst(bytes, header)
//	require.NoError(t, err)
//	t.Logf("%+v", header) // &{Tag:0 ID:0}
//
//	payload2 := &P2pkhPayload{}
//	err = cbor.Unmarshal(bytes, payload2)
//	require.NoError(t, err)
//	t.Logf("%+v", payload2) // &{Header:0xc0000b4000 Hash:[2]}
//}
//
//func Test1(t *testing.T) {
//	//cbor.
//}
//
////{"Tag": 16, "ID": 1, "Hash": h'01'}
//// with array on header: [16, 1, h'01']
//// with array on both: [16, 1, h'01']
//// with array on outer: [16, 1, h'01']
