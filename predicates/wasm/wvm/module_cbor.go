package wvm

import (
	"context"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/predicates/wasm/wvm/encoder"
)

/*
CBOR support APIs
*/
func addCBORModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("cbor").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(cborParse), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("parse").
		// not used by the demo anymore so do we want to have such API?
		//NewFunctionBuilder().WithGoModuleFunction(hostAPI(cbor_parse_array_raw), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("cbor_parse_array").
		Instantiate(ctx)
	return err
}

/*
encodes plain old data types only!
- 0: handle of var
*/
func cborParse(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[0])
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	var items any
	if err := types.Cbor.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decoding as CBOR: %w", err)
	}

	var enc encoder.TVEnc
	enc.Encode(items)
	buf, err := enc.Bytes()
	if err != nil {
		return fmt.Errorf("encoding data to type-value: %w", err)
	}

	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for result: %w", err)
	}
	stack[0] = addr
	return nil
}

/*
cborParseArrayRaw, given handle of an raw CBOR buffer (stack[0]) parses it as
array of raw CBOR items. Returns list of (item) handles.
*/
func cborParseArrayRaw(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[0])
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	var items []types.RawCBOR
	if err := types.Cbor.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decoding arguments as array of CBOR: %w", err)
	}

	// add items as new vars and return array of uint64 handles
	var enc encoder.TVEnc
	enc.WriteUInt32(uint32(len(items)))
	for _, v := range items {
		enc.WriteUInt64(vec.curPrg.addVar(v))
	}
	buf, err := enc.Bytes()
	if err != nil {
		return fmt.Errorf("encoding result: %w", err)
	}
	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for result: %w", err)
	}
	stack[0] = addr
	return nil
}
