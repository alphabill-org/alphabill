package wvm

import (
	_ "embed"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/allocator"
)

func Test_cbor_parse_array(t *testing.T) {
	vm := &VmContext{
		curPrg: &EvalContext{
			// the "txProof" contains tx proof CBOR as saved by CLI wallet:
			// array of array pairs [ {[txRec],[txProof]}, {...} ]
			vars:   map[uint64]any{handle_current_args: txProof},
			varIdx: handle_max_reserved,
		},
		factory: ABTypesFactory{},
		MemMngr: allocator.NewBumpAllocator(0, maxMem(10000)),
	}

	var handle, handle2 uint64

	mem := &mockMemory{
		size: func() uint32 { return 10000 },
		// write is called for the memory "sent back" to the module
		// ie it contains the response - array of item handles
		write: func(offset uint32, v []byte) bool {
			require.Len(t, v, 4+8) // we expect length (uint32) and single uint64
			require.EqualValues(t, 1, binary.LittleEndian.Uint32(v))
			handle = binary.LittleEndian.Uint64(v[4:])
			return true
		},
	}
	mod := &mockApiMod{memory: func() api.Memory { return mem }}

	// outer container, handle points to array with single item
	require.NoError(t, cbor_parse_array(vm, mod, []uint64{handle_current_args}))
	require.NotZero(t, handle)

	// the item we extracted must be itself an array of two items
	// hack: to capture handles replace module's memory write mock
	mem.write = func(offset uint32, v []byte) bool {
		require.Len(t, v, 4+2*8) // we expect two uint64 handles
		require.EqualValues(t, 2, binary.LittleEndian.Uint32(v))
		handle = binary.LittleEndian.Uint64(v[4:])
		handle2 = binary.LittleEndian.Uint64(v[4+8:])
		return true
	}
	require.NoError(t, cbor_parse_array(vm, mod, []uint64{handle}))
	// handle and handle2 should now point to raw CBOR of txRecord and txProof respectively
	buf, err := getVar[types.RawCBOR](vm.curPrg.vars, handle)
	require.NoError(t, err)
	txRec := &types.TransactionRecord{}
	require.NoError(t, types.Cbor.Unmarshal(buf, txRec))

	buf, err = getVar[types.RawCBOR](vm.curPrg.vars, handle2)
	require.NoError(t, err)
	txProof := &types.TxProof{}
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
	// to "implement" everything we haven't mocked
	api.Memory
}

func (m *mockMemory) Size() uint32                       { return m.size() }
func (m *mockMemory) Write(offset uint32, v []byte) bool { return m.write(offset, v) }

type maxMem uint32

func (m maxMem) Max() (uint32, bool) { return uint32(m), true }

// raw CBOR as saved by CLI wallet's "save tx proof" flag.
// Outer array has single item which is two arrays (txRecord and txProof)
//
//go:embed testdata/proof.cbor
var txProof []byte
