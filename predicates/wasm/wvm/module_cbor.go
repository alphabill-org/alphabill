package wvm

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/cbor"
)

/*
CBOR support APIs
*/
func addCBORModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("cbor").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(cborParse), []api.ValueType{api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("parse").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(cborChunks), []api.ValueType{api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("chunks").
		Instantiate(ctx)
	return err
}

/*
cborParse decodes referenced var as CBOR and stores the result into new (host) variable(s).
Returns handle(s) to the new variable(s).
  - 0: handle of the var to parse
  - 1: flag - parse as array (0) or single struct (1)?
*/
func cborParse(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(api.DecodeU32(stack[0]))
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	var items any
	if err := cbor.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decoding as CBOR: %w", err)
	}

	// should we tread it as array or as struct (AB encodes structs as arrays too)
	asArray := api.DecodeU32(stack[1]) == 0
	var buf []byte
	if a, ok := items.([]any); ok && asArray {
		buf = make([]byte, 0, 4*len(a))
		for _, v := range a {
			buf = binary.NativeEndian.AppendUint32(buf, vec.curPrg.addVar(v))
		}
	} else {
		buf = make([]byte, 4)
		binary.NativeEndian.PutUint32(buf, vec.curPrg.addVar(items))
	}

	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for result: %w", err)
	}
	vec.log.Debug(fmt.Sprintf("%x => %v @ %x", api.DecodeU32(stack[0]), buf, addr))
	stack[0] = addr
	return nil
}

/*
Parse CBOR array to "raw chunks" ie all items will still be CBOR encoded.
*/
func cborChunks(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(api.DecodeU32(stack[0]))
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	var items []cbor.RawCBOR
	if err := cbor.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decoding as CBOR: %w", err)
	}

	buf := make([]byte, 0, 4*len(items))
	for _, v := range items {
		buf = binary.NativeEndian.AppendUint32(buf, vec.curPrg.addVar(v))
	}

	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for result: %w", err)
	}
	stack[0] = addr
	return nil
}
