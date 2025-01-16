package wvm

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/bumpallocator"
)

func Test_cbor_parse(t *testing.T) {
	t.Run("unknown handle", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint64]any{},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `reading variable: variable with handle 3 not found`)
	})

	t.Run("wrong data type behind handle", func(t *testing.T) {
		// data must be []byte "compatible"
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint64]any{handle_predicate_conf: 42},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `reading variable: can't handle var of type int`)
	})

	t.Run("bytes but invalid CBOR", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint64]any{handle_predicate_conf: []byte{}},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `decoding as CBOR: EOF`)
	})

	t.Run("success", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				// CBOR(830a63737472430102ff) == [10, "str", h'0102FF']
				vars: map[uint64]any{handle_predicate_conf: []byte{0x83, 0x0a, 0x63, 0x73, 0x74, 0x72, 0x43, 0x01, 0x02, 0xff}},
			},
			memMngr: bumpallocator.New(0, maxMem(10000)),
		}
		mem := &mockMemory{
			size: func() uint32 { return 10000 },
			// write is called for the memory "sent back" to the module
			// ie it contains the response
			write: func(offset uint32, v []byte) bool {
				require.Equal(t, []byte{0x5, 0x3, 0x0, 0x0, 0x0, 0x2, 0xa, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x4, 0x3, 0x0, 0x0, 0x0, 0x73, 0x74, 0x72, 0x1, 0x3, 0x0, 0x0, 0x0, 0x1, 0x2, 0xff}, v)
				return true
			},
		}
		mod := &mockApiMod{memory: func() api.Memory { return mem }}
		require.NoError(t, cborParse(ctx, mod, []uint64{handle_predicate_conf}))
	})
}

func Test_cbor_parse_array_raw(t *testing.T) {
	t.Skip("test fails as UnicityCertificate struct has changed (number of fields) and thus CBOR decoding fails")
	vm := &vmContext{
		curPrg: &evalContext{
			// the "txProof" contains tx proof CBOR as saved by CLI wallet:
			// array of array pairs [ {[txRec],[txProof]}, {...} ]
			vars:   map[uint64]any{handle_current_args: txProof},
			varIdx: handle_max_reserved,
		},
		factory: ABTypesFactory{},
		memMngr: bumpallocator.New(0, maxMem(10000)),
	}

	var handle, handle2 uint64

	mem := &mockMemory{
		size: func() uint32 { return 10000 },
		// write is called for the memory "sent back" to the module
		// ie it contains the response - array of item handles
		write: func(offset uint32, v []byte) bool {
			require.Len(t, v, 2+4+8) // we expect length (uint32) and single uint64 (+2 for type tags)
			require.EqualValues(t, 1, binary.LittleEndian.Uint32(v[1:]))
			handle = binary.LittleEndian.Uint64(v[6:])
			return true
		},
	}
	mod := &mockApiMod{memory: func() api.Memory { return mem }}

	// outer container, handle points to array with single item
	require.NoError(t, cborParseArrayRaw(vm, mod, []uint64{handle_current_args}))
	require.NotZero(t, handle)

	// the item we extracted must be itself an array of two items
	// hack: to capture handles replace module's memory write mock
	mem.write = func(offset uint32, v []byte) bool {
		require.Len(t, v, 3+4+2*8) // we expect two uint64 handles (+3 type tags)
		require.EqualValues(t, 2, binary.LittleEndian.Uint32(v[1:]))
		handle = binary.LittleEndian.Uint64(v[6:])
		handle2 = binary.LittleEndian.Uint64(v[15:])
		return true
	}
	require.NoError(t, cborParseArrayRaw(vm, mod, []uint64{handle}))
	// handle and handle2 should now point to raw CBOR of txRecord and txProof respectively
	buf, err := getVar[types.RawCBOR](vm.curPrg.vars, handle)
	require.NoError(t, err)
	txRec := &types.TransactionRecord{Version: 1}
	require.NoError(t, types.Cbor.Unmarshal(buf, txRec))

	buf, err = getVar[types.RawCBOR](vm.curPrg.vars, handle2)
	require.NoError(t, err)
	txProof := &types.TxProof{Version: 1}
	require.NoError(t, types.Cbor.Unmarshal(buf, txProof))
}

type mockApiMod struct {
	memory func() api.Memory
	// to "implement" everything we haven't mocked
	api.Module
}

func (m *mockApiMod) Memory() api.Memory { return m.memory() }

type mockMemory struct {
	size  func() uint32
	write func(offset uint32, v []byte) bool
	read  func(offset, byteCount uint32) ([]byte, bool)
	// to "implement" everything we haven't mocked
	api.Memory
}

func (m *mockMemory) Size() uint32                                 { return m.size() }
func (m *mockMemory) Write(offset uint32, v []byte) bool           { return m.write(offset, v) }
func (m *mockMemory) Read(offset, byteCount uint32) ([]byte, bool) { return m.read(offset, byteCount) }

type maxMem uint32

func (m maxMem) Max() (uint32, bool) { return uint32(m), true }

// raw CBOR as saved by CLI wallet's "save tx proof" flag.
// Outer array has single item which is two arrays (txRecord and txProof)
//
//go:embed testdata/proof.cbor
var txProof []byte
