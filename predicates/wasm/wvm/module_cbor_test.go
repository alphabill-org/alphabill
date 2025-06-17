package wvm

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/cbor"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/bumpallocator"
)

func Test_cborParse(t *testing.T) {
	t.Run("unknown handle", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint32]any{},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `reading variable: variable with handle 3 not found`)
	})

	t.Run("wrong data type behind handle", func(t *testing.T) {
		// data must be []byte "compatible"
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint32]any{handle_predicate_conf: 42},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `reading variable: can't handle var of type int`)
	})

	t.Run("bytes but invalid CBOR", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				vars: map[uint32]any{handle_predicate_conf: []byte{}},
			},
		}
		err := cborParse(ctx, nil, []uint64{handle_predicate_conf})
		require.EqualError(t, err, `decoding as CBOR: EOF`)
	})

	t.Run("success, struct", func(t *testing.T) {
		ctx := &vmContext{
			curPrg: &evalContext{
				// CBOR(830a63737472430102ff) == [10, "str", h'0102FF']
				vars:   map[uint32]any{handle_predicate_conf: []byte{0x83, 0x0a, 0x63, 0x73, 0x74, 0x72, 0x43, 0x01, 0x02, 0xff}},
				varIdx: handle_max_reserved,
			},
			memMngr: bumpallocator.New(0, maxMem(10000)),
			log:     logger.New(t),
		}
		var handle uint32
		mem := &mockMemory{
			size: func() uint32 { return 10000 },
			// write is called for the memory "sent back" to the module
			// ie it contains the response
			write: func(offset uint32, v []byte) bool {
				handle = binary.NativeEndian.Uint32(v)
				return true
			},
		}
		mod := &mockApiMod{memory: func() api.Memory { return mem }}
		// parse conf and return handle to it
		stack := []uint64{handle_predicate_conf, 1}
		require.NoError(t, cborParse(ctx, mod, stack))
		// the parsed data was stored into new var
		v := ctx.curPrg.vars[handle]
		require.Equal(t, []any{uint64(10), "str", []byte{1, 2, 255}}, v)
	})

	t.Run("success, array", func(t *testing.T) {
		conf, err := cbor.Marshal([]any{1234567890, []byte{1, 2, 3, 4, 5}})
		require.NoError(t, err)
		ctx := &vmContext{
			curPrg: &evalContext{
				vars:   map[uint32]any{handle_predicate_conf: conf},
				varIdx: handle_max_reserved,
			},
			memMngr: bumpallocator.New(0, maxMem(10000)),
			log:     logger.New(t),
		}
		var handles [2]uint32
		mem := &mockMemory{
			size: func() uint32 { return 10000 },
			// write is called for the memory "sent back" to the module
			// ie it contains the response
			write: func(offset uint32, v []byte) bool {
				require.Len(t, v, 2*4, "expected byte slice of two uint32")
				handles[0] = binary.NativeEndian.Uint32(v[0:4])
				handles[1] = binary.NativeEndian.Uint32(v[4:8])
				return true
			},
		}
		mod := &mockApiMod{memory: func() api.Memory { return mem }}
		// parse conf and return handle to it
		stack := []uint64{handle_predicate_conf, 0}
		require.NoError(t, cborParse(ctx, mod, stack))
		// the parsed data was stored into two new var
		v := ctx.curPrg.vars[handles[0]]
		require.Equal(t, uint64(1234567890), v)
		v = ctx.curPrg.vars[handles[1]]
		require.Equal(t, []byte{1, 2, 3, 4, 5}, v)
	})
}

func Test_cborChunks(t *testing.T) {
	cborData, err := cbor.Marshal([]any{1234567890, []byte{1, 2, 3, 4, 5}})
	require.NoError(t, err)
	vm := &vmContext{
		curPrg: &evalContext{
			vars:   map[uint32]any{handle_current_args: cborData},
			varIdx: handle_max_reserved,
		},
		factory: ABTypesFactory{},
		memMngr: bumpallocator.New(0, maxMem(10000)),
	}

	var handles [2]uint32

	mem := &mockMemory{
		size: func() uint32 { return 10000 },
		// write is called for the memory "sent back" to the module
		// ie it contains the response - array of item handles
		write: func(offset uint32, v []byte) bool {
			require.Len(t, v, 2*4, "expected byte slice of two uint32")
			handles[0] = binary.NativeEndian.Uint32(v[0:4])
			handles[1] = binary.NativeEndian.Uint32(v[4:8])
			return true
		},
	}
	mod := &mockApiMod{memory: func() api.Memory { return mem }}

	// outer container, handle points to array with single item
	require.NoError(t, cborChunks(vm, mod, []uint64{handle_current_args}))
	require.NotZero(t, handles[0])
	require.NotZero(t, handles[1])

	// the first handle is for cbor encoded uint64
	var v any
	buf, err := getVar[cbor.RawCBOR](vm.curPrg.vars, handles[0])
	require.NoError(t, err)
	require.NoError(t, cbor.Unmarshal(buf, &v))
	require.Equal(t, uint64(1234567890), v)

	// the second handle is for cbor encoded byte slice
	buf, err = getVar[cbor.RawCBOR](vm.curPrg.vars, handles[1])
	require.NoError(t, err)
	require.NoError(t, cbor.Unmarshal(buf, &v))
	require.Equal(t, []byte{1, 2, 3, 4, 5}, v)
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
