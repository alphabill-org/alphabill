package instrument

/*
#cgo linux,amd64 LDFLAGS: -Wl,--no-as-needed -ldl ${SRCDIR}/wasm-instrument-rust/lib/x86_64-unknown-linux-gnu/libwasm_instrument.a
#cgo darwin,arm64 LDFLAGS: ${SRCDIR}/wasm-instrument-rust/lib/aarch64-apple-darwin/libwasm_instrument.a
#cgo darwin,amd64 LDFLAGS: ${SRCDIR}/wasm-instrument-rust/lib/x86_64-apple-darwin/libwasm_instrument.a
#include "./wasm-instrument-rust/include/instrument_wasm.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

const GasCounter string = "gas_count"

// MeterGasAndStack - adds gas and stack instrumentation.
// gas instrumentation uses global counter "gas_count"
// stack metering is based on input, if 0 then no stack metering is inserted
func MeterGasAndStack(wasm []byte, stackHeight uint32) ([]byte, error) {
	var wasmBytes C.MemoryBuffer
	wasmLen := len(wasm)
	C.memory_buffer_new(&wasmBytes, C.size_t(wasmLen), (*C.uchar)(C.CBytes(wasm)))
	defer C.memory_buffer_delete(&wasmBytes)
	var wasmRes C.MemoryBuffer
	res := C.instrument_wasm(&wasmBytes, C.uint(stackHeight), &wasmRes)
	if res != 0 {
		// get error details
		cerr := C.errstr()
		if cerr == nil {
			return nil, fmt.Errorf("wasm instrumentation error")
		}
		defer C.errstr_free(cerr)
		errorStr := C.GoString(cerr)
		return nil, errors.New(errorStr)
	}
	defer C.memory_buffer_delete(&wasmRes)
	result := C.GoBytes(unsafe.Pointer(wasmRes.data), C.int(wasmRes.size))
	return result, nil
}
