package tokenenc

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/txsystem/tokens"
	"github.com/alphabill-org/alphabill-go-base/types"
)

func Test_tokenAttributesEncoding_trigger(t *testing.T) {
	/*
		If test here fails it's probably because some data structure (or rather
		how it's serialized for Rust SDK) has been changed without versioning?
		Also, the Rust predicates SDK likely needs to be updated!
		See the other tests here to generate tests for Rust SDK.
	*/

	type encAttr struct {
		attr any
		enc  []byte
	}

	t.Run("CreateNonFungibleTokenTypeAttributes", func(t *testing.T) {
		tests := []encAttr{
			{
				attr: tokens.DefineNonFungibleTokenAttributes{
					Symbol: "AB",
					Name:   "test token",
				},
				enc: []byte{0x1, 0x5, 0x2, 0x0, 0x0, 0x0, 0x41, 0x42, 0x2, 0x5, 0xa, 0x0, 0x0, 0x0, 0x74, 0x65, 0x73, 0x74, 0x20, 0x74, 0x6f, 0x6b, 0x65, 0x6e},
			},
			{
				attr: tokens.DefineNonFungibleTokenAttributes{
					ParentTypeID: []byte{1, 2, 3, 8, 9, 0},
					Symbol:       "AB-NFT",
					Name:         "funky token",
				},
				enc: []byte{0x1, 0x5, 0x6, 0x0, 0x0, 0x0, 0x41, 0x42, 0x2d, 0x4e, 0x46, 0x54, 0x2, 0x5, 0xb, 0x0, 0x0, 0x0, 0x66, 0x75, 0x6e, 0x6b, 0x79, 0x20, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x3, 0x1, 0x6, 0x0, 0x0, 0x0, 0x1, 0x2, 0x3, 0x8, 0x9, 0x0},
			},
		}
		txo := &types.TransactionOrder{Payload: types.Payload{}}
		for n, tc := range tests {
			require.NoError(t, txo.SetAttributes(tc.attr))
			b, err := txaCreateNonFungibleTokenTypeAttributes(txo, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})

	t.Run("MintNonFungibleTokenAttributes", func(t *testing.T) {
		tests := []encAttr{
			{
				attr: tokens.MintNonFungibleTokenAttributes{
					TypeID: []byte{8, 7, 6, 5},
					Name:   "test token",
					Nonce:  1,
				},
				enc: []byte{0x1, 0x5, 0xa, 0x0, 0x0, 0x0, 0x74, 0x65, 0x73, 0x74, 0x20, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x4, 0x2, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x1, 0x4, 0x0, 0x0, 0x0, 0x8, 0x7, 0x6, 0x5},
			},
			{
				attr: tokens.MintNonFungibleTokenAttributes{
					TypeID: []byte{255, 255, 255},
					Name:   "test token",
					Nonce:  1000,
					URI:    "ab://nft/token",
					Data:   []byte("data!"),
				},
				enc: []byte{0x1, 0x5, 0xa, 0x0, 0x0, 0x0, 0x74, 0x65, 0x73, 0x74, 0x20, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x2, 0x5, 0xe, 0x0, 0x0, 0x0, 0x61, 0x62, 0x3a, 0x2f, 0x2f, 0x6e, 0x66, 0x74, 0x2f, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x3, 0x1, 0x5, 0x0, 0x0, 0x0, 0x64, 0x61, 0x74, 0x61, 0x21, 0x4, 0x2, 0xe8, 0x3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x5, 0x1, 0x3, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff},
			},
		}
		txo := &types.TransactionOrder{Payload: types.Payload{}}
		for n, tc := range tests {
			require.NoError(t, txo.SetAttributes(tc.attr))
			b, err := txaMintNonFungibleTokenAttributes(txo, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})

	t.Run("TransferNonFungibleTokenAttributes", func(t *testing.T) {
		tests := []encAttr{
			{
				attr: tokens.TransferNonFungibleTokenAttributes{
					TypeID:  []byte{8, 7, 6, 5},
					Counter: 7,
				},
				enc: []byte{0x1, 0x1, 0x4, 0x0, 0x0, 0x0, 0x8, 0x7, 0x6, 0x5, 0x2, 0x2, 0x7, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
		}
		txo := &types.TransactionOrder{Payload: types.Payload{}}
		for n, tc := range tests {
			require.NoError(t, txo.SetAttributes(tc.attr))
			b, err := txaTransferNonFungibleTokenAttributes(txo, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})

	t.Run("UpdateNonFungibleTokenAttributes", func(t *testing.T) {
		tests := []encAttr{
			{
				attr: tokens.UpdateNonFungibleTokenAttributes{
					Data:    []byte{},
					Counter: 6,
				},
				enc: []byte{0x1, 0x1, 0x0, 0x0, 0x0, 0x0, 0x2, 0x2, 0x6, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			{
				attr: tokens.UpdateNonFungibleTokenAttributes{
					Data:    []byte("new data here"),
					Counter: 7,
				},
				enc: []byte{0x1, 0x1, 0xd, 0x0, 0x0, 0x0, 0x6e, 0x65, 0x77, 0x20, 0x64, 0x61, 0x74, 0x61, 0x20, 0x68, 0x65, 0x72, 0x65, 0x2, 0x2, 0x7, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
		}
		txo := &types.TransactionOrder{Payload: types.Payload{}}
		for n, tc := range tests {
			require.NoError(t, txo.SetAttributes(tc.attr))
			b, err := txaUpdateNonFungibleTokenAttributes(txo, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})

	// unit data types

	type utEnc struct {
		data types.UnitData
		enc  []byte
	}

	t.Run("NonFungibleTokenData", func(t *testing.T) {
		tests := []utEnc{
			{
				data: &tokens.NonFungibleTokenData{
					TypeID:  []byte{1, 5, 0},
					T:       32,
					Counter: 90,
					Locked:  0,
				},
				enc: []byte{0x1, 0x1, 0x3, 0x0, 0x0, 0x0, 0x1, 0x5, 0x0, 0x5, 0x2, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6, 0x2, 0x5a, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
			{
				data: &tokens.NonFungibleTokenData{
					TypeID:  []byte{1},
					Name:    "hot stuff",
					URI:     "foo/bar",
					Data:    []byte{9, 1, 1},
					T:       32,
					Counter: 90,
					Locked:  1,
				},
				enc: []byte{0x1, 0x1, 0x1, 0x0, 0x0, 0x0, 0x1, 0x2, 0x5, 0x9, 0x0, 0x0, 0x0, 0x68, 0x6f, 0x74, 0x20, 0x73, 0x74, 0x75, 0x66, 0x66, 0x3, 0x5, 0x7, 0x0, 0x0, 0x0, 0x66, 0x6f, 0x6f, 0x2f, 0x62, 0x61, 0x72, 0x4, 0x1, 0x3, 0x0, 0x0, 0x0, 0x9, 0x1, 0x1, 0x5, 0x2, 0x20, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6, 0x2, 0x5a, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x7, 0x2, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0},
			},
		}
		for n, tc := range tests {
			b, err := udeNonFungibleTokenData(tc.data, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})

	t.Run("NonFungibleTokenTypeData", func(t *testing.T) {
		tests := []utEnc{
			{
				data: &tokens.NonFungibleTokenTypeData{
					ParentTypeID: nil,
					Symbol:       "TT",
					Name:         "name of the type",
				},
				enc: []byte{0x2, 0x5, 0x2, 0x0, 0x0, 0x0, 0x54, 0x54, 0x3, 0x5, 0x10, 0x0, 0x0, 0x0, 0x6e, 0x61, 0x6d, 0x65, 0x20, 0x6f, 0x66, 0x20, 0x74, 0x68, 0x65, 0x20, 0x74, 0x79, 0x70, 0x65},
			},
			{
				data: &tokens.NonFungibleTokenTypeData{
					ParentTypeID: []byte{1, 5, 0},
					Symbol:       "-@!",
					Name:         "funny",
				},
				enc: []byte{0x1, 0x1, 0x3, 0x0, 0x0, 0x0, 0x1, 0x5, 0x0, 0x2, 0x5, 0x3, 0x0, 0x0, 0x0, 0x2d, 0x40, 0x21, 0x3, 0x5, 0x5, 0x0, 0x0, 0x0, 0x66, 0x75, 0x6e, 0x6e, 0x79},
			},
		}
		for n, tc := range tests {
			b, err := udeNonFungibleTokenTypeData(tc.data, 1)
			require.NoError(t, err, "test case %d", n)
			require.Equal(t, tc.enc, b, "test case %d", n)
		}
	})
}

func Test_generateUnitDataDecodeTests(t *testing.T) {
	t.Skip("generate test data for Rust predicate SDK - decode token unit data")
	/*
		To test that the Rust SDK is able to decode token tx data generated by the host
		we generate these Rust tests:
		- set the (absolute) path in the os.Create statement below to where to save the
		  generated code;
		- comment out the Skip statement in the beginning;
		- run the test(s);
		- copy the generated code into src/txsystem/nft.rs in Rust SDK (manage the
		  different versions of the data in Rust side!);
		- NB! the test fails on purpose to remind you to enable the Skip statement again!

		At this stage doing all this manual work seems to be acceptable (rather than building
		some test harness to do it)...
	*/

	fOut, err := os.Create("/Users/ain/go/src/alphabill/txsystem/tokens/encoder/decoder_test.rs")
	require.NoError(t, err)
	defer fOut.Close()
	fmt.Fprint(fOut, "DO NOT CHECK THIS FILE INTO REPOSITORY - IT'S MEANT FOR GENERATING TESTS FOR RUST PREDICATES PROJECT\n\n")

	type inOut struct {
		attr   types.UnitData
		fields func(attr any) string
	}

	generateTest := func(t *testing.T, encoder func(data types.UnitData, ver uint32) ([]byte, error), version uint32, typeName string, tests []inOut) {
		out := bytes.NewBufferString("\n#[test]\n#[allow(non_snake_case)]\n")
		out.WriteString(fmt.Sprintf("fn %s_from_%d() {\n", typeName, version))
		out.WriteString("// test generated by Go backend!\n")
		for _, td := range tests {
			buf, err := encoder(td.attr, version)
			require.NoError(t, err)
			out.WriteString("let data = vec![" + bytesAsHex(t, buf) + "];\n")
			out.WriteString("assert_eq!(" + typeName + "::from(data).unwrap(),\n" + typeName + "{" + td.fields(td.attr) + "}\n);\n")
		}
		out.WriteString("}\n")

		_, err = out.WriteTo(fOut)
		require.NoError(t, err)
	}

	t.Run("NFT token data", func(t *testing.T) {
		// version 1 encoding
		const version = 1
		fmtFields := func(a any) string {
			attr := a.(*tokens.NonFungibleTokenData)
			return fieldList(t,
				"type_id", rustOptionValue(t, attr.TypeID),
				"name", rustOptionValue(t, attr.Name),
				"uri", rustOptionValue(t, attr.URI),
				"data", rustOptionValue(t, attr.Data),
				"last_round", rustOptionValue(t, attr.T),
				"counter", rustOptionValue(t, attr.Counter),
				"locked", fmt.Sprintf("Some(%d)", attr.Locked),
			)
		}
		tests := []inOut{
			{
				attr: &tokens.NonFungibleTokenData{
					TypeID:  []byte{1, 5, 0},
					T:       32,
					Counter: 90,
					Locked:  0,
				},
				fields: fmtFields,
			},
			{
				attr: &tokens.NonFungibleTokenData{
					TypeID:  []byte{1},
					Name:    "hot stuff",
					URI:     "foo/bar",
					Data:    []byte{9, 1, 1},
					T:       32,
					Counter: 90,
					Locked:  1,
				},
				fields: fmtFields,
			},
		}

		generateTest(t, udeNonFungibleTokenData, version, "TokenData", tests)
	})

	t.Run("NFT type data", func(t *testing.T) {
		// version 1 encoding
		const version = 1
		fmtFields := func(a any) string {
			attr := a.(*tokens.NonFungibleTokenTypeData)
			return fieldList(t,
				"parent_id", rustOptionValue(t, attr.ParentTypeID),
				"symbol", rustOptionValue(t, attr.Symbol),
				"name", rustOptionValue(t, attr.Name),
			)
		}
		tests := []inOut{
			{
				attr: &tokens.NonFungibleTokenTypeData{
					Name:   "abcde",
					Symbol: "A",
				},
				fields: fmtFields,
			},
			{
				attr: &tokens.NonFungibleTokenTypeData{
					ParentTypeID: []byte{255, 0, 127, 128},
					Name:         "qwerty",
					Symbol:       "oh!",
				},
				fields: fmtFields,
			},
		}

		generateTest(t, udeNonFungibleTokenTypeData, version, "TypeData", tests)
	})

	t.Error("Do NOT remove this error - instead add/enable the t.Skip statement in the beginning of the test! " +
		"This error is here so that the test is enabled only for re-generating the test code for Rust SDK and then disabled again.")
}

func Test_generateTxAttributeDecodeTests(t *testing.T) {
	t.Skip("generate test data for Rust predicate SDK - decode token TxAttributes")
	/*
		To test that the Rust SDK is able to decode token tx data generated by the host
		we generate these Rust tests:
		- set the (absolute) path in the os.Create statement below to where to save the
		  generated code;
		- comment out the Skip statement in the beginning;
		- run the test(s);
		- copy the generated code into src/txsystem/nft.rs in Rust SDK (manage the
		  different versions of the data in Rust side!);
		- NB! the test fails on purpose to remind you to enable the Skip statement again!

		At this stage doing all this manual work seems to be acceptable (rather than building
		some test harness to do it)...
	*/

	fOut, err := os.Create("decoder_test.rs")
	require.NoError(t, err)
	defer fOut.Close()
	fmt.Fprint(fOut, "DO NOT CHECK THIS FILE INTO REPOSITORY - IT'S MEANT FOR GENERATING TESTS FOR RUST PREDICATES PROJECT\n\n")

	type inOut struct {
		attr   any
		fields func(attr any) string
	}

	generateTest := func(t *testing.T, encoder func(txo *types.TransactionOrder, ver uint32) ([]byte, error), version uint32, typeName string, tests []inOut) {
		txo := types.TransactionOrder{Payload: types.Payload{}}
		out := bytes.NewBufferString("\n#[test]\n#[allow(non_snake_case)]\n")
		out.WriteString(fmt.Sprintf("fn %s_from_%d() {\n", typeName, version))
		out.WriteString("// test generated by Go backend!\n")
		for _, td := range tests {
			require.NoError(t, txo.SetAttributes(td.attr))
			buf, err := encoder(&txo, version)
			require.NoError(t, err)
			out.WriteString("let data = vec![" + bytesAsHex(t, buf) + "];\n")
			out.WriteString("assert_eq!(" + typeName + "::from(data).unwrap(),\n" + typeName + "{" + td.fields(td.attr) + "}\n);\n")
		}
		out.WriteString("}\n")

		_, err = out.WriteTo(fOut)
		require.NoError(t, err)
	}

	t.Run("NFT mint type", func(t *testing.T) {
		// version 1 encoding exports: Symbol, Name, ParentTypeID (optional)
		const version = 1
		fmtFields := func(a any) string {
			attr := a.(tokens.DefineNonFungibleTokenAttributes)
			return fieldList(t,
				"name", rustOptionValue(t, attr.Name),
				"symbol", rustOptionValue(t, attr.Symbol),
				"type_id", rustOptionValue(t, attr.ParentTypeID),
			)
		}
		tests := []inOut{
			{
				attr: tokens.DefineNonFungibleTokenAttributes{
					Symbol: "AB",
					Name:   "test token",
				},
				fields: fmtFields,
			},
			{
				attr: tokens.DefineNonFungibleTokenAttributes{
					ParentTypeID: []byte{1, 2, 3, 8, 9, 0},
					Symbol:       "AB-NFT",
					Name:         "funky token",
				},
				fields: fmtFields,
			},
		}

		generateTest(t, txaCreateNonFungibleTokenTypeAttributes, version, "CreateType", tests)
	})

	t.Run("NFT mint", func(t *testing.T) {
		// version 1 encoding exports: type_id, name, nonce, uri, data, last two are optional
		const version = 1 // the version of the encoding of this type
		fmtFields := func(a any) string {
			attr := a.(tokens.MintNonFungibleTokenAttributes)
			return fieldList(t,
				"name", rustOptionValue(t, attr.Name),
				"uri", rustOptionValue(t, attr.URI),
				"data", rustOptionValue(t, attr.Data),
				"nonce", rustOptionValue(t, attr.Nonce),
				"type_id", rustOptionValue(t, attr.TypeID),
			)
		}
		tests := []inOut{
			{
				attr: tokens.MintNonFungibleTokenAttributes{
					TypeID: []byte{8, 7, 6, 5},
					Name:   "test token",
					Nonce:  1,
				},
				fields: fmtFields,
			},
			{
				attr: tokens.MintNonFungibleTokenAttributes{
					TypeID: []byte{255, 255, 255},
					Name:   "test token",
					Nonce:  1000,
					URI:    "ab://nft/token",
					Data:   []byte("data!"),
				},
				fields: fmtFields,
			},
		}

		generateTest(t, txaMintNonFungibleTokenAttributes, version, "Mint", tests)
	})

	t.Run("NFT transfer", func(t *testing.T) {
		// version 1 encoding exports: type_id, counter
		const version = 1 // the version of the encoding of this type
		fmtFields := func(a any) string {
			attr := a.(tokens.TransferNonFungibleTokenAttributes)
			return fieldList(t,
				"counter", rustOptionValue(t, attr.Counter),
				"type_id", rustOptionValue(t, attr.TypeID),
			)
		}
		tests := []inOut{
			{
				attr: tokens.TransferNonFungibleTokenAttributes{
					TypeID:  []byte{8, 7, 6, 5},
					Counter: 7,
				},
				fields: fmtFields,
			},
			{
				attr: tokens.TransferNonFungibleTokenAttributes{
					TypeID:  []byte{8, 7, 6, 5},
					Counter: 7,
				},
				fields: fmtFields,
			},
		}

		generateTest(t, txaTransferNonFungibleTokenAttributes, version, "Transfer", tests)
	})

	t.Run("NFT update", func(t *testing.T) {
		// version 1 encoding exports: data, counter
		const version = 1 // the version of the encoding of this type
		fmtFields := func(a any) string {
			attr := a.(tokens.UpdateNonFungibleTokenAttributes)
			return fieldList(t,
				"counter", rustOptionValue(t, attr.Counter),
				"data", rustOptionValue(t, attr.Data),
			)
		}
		tests := []inOut{
			{
				attr: tokens.UpdateNonFungibleTokenAttributes{
					Data:    []byte{},
					Counter: 6,
				},
				fields: fmtFields,
			},
			{
				attr: tokens.UpdateNonFungibleTokenAttributes{
					Data:    []byte("new data here"),
					Counter: 7,
				},
				fields: fmtFields,
			},
		}

		generateTest(t, txaUpdateNonFungibleTokenAttributes, version, "Update", tests)
	})

	t.Error("Do NOT remove this error - instead add/enable the t.Skip statement in the beginning of the test! " +
		"This error is here so that the test is enabled only for re-generating the test code for Rust SDK and then disabled again.")
}

func fieldList(t *testing.T, flds ...string) string {
	t.Helper()
	if len(flds)%2 != 0 {
		t.Fatalf("fields - values must be even number, got %d", len(flds))
	}

	r := ""
	for i := 0; i < len(flds); i += 2 {
		r += flds[i] + ": " + flds[i+1] + ", "
	}
	return r
}

// v as Rust SDK Value enum
func rustOptionValue(t *testing.T, v any) string {
	t.Helper()
	switch tv := v.(type) {
	case nil:
		return "None"
	case []byte:
		switch {
		case tv == nil:
			return "None"
		case len(tv) == 0:
			return "Some(Vec::new())"
		default:
			return fmt.Sprintf("Some(vec![%s])", bytesAsHex(t, tv))
		}
	case types.UnitID:
		if tv == nil {
			return "None"
		}
		return fmt.Sprintf("Some(vec![%s])", bytesAsHex(t, tv))
	case string:
		if tv == "" {
			return "None"
		}
		return fmt.Sprintf("Some(%q.to_string())", tv)
	case uint64:
		if tv == 0 {
			return "None"
		}
		return fmt.Sprintf("Some(%d)", tv)
	default:
		t.Errorf("unsupported type %T", v)
		return ""
	}
}

// byte slice in Rust syntax (without enclosing [])
func bytesAsHex(t *testing.T, b []byte) string {
	t.Helper()
	out := bytes.Buffer{}
	for _, v := range b {
		out.WriteString(fmt.Sprintf("0x%x, ", v))
	}
	return strings.TrimSuffix(out.String(), ", ")
}
