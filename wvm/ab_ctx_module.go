package wvm

import (
	"context"
	"fmt"

	"github.com/fxamacker/cbor/v2"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/wvm/encoder"
)

/*
addContextModule adds "context" module to the "rt".
This module provides access to "current predicate evaluation context", ie current transaction,
predicate arguments, round number etc.

encoder and factory as arguments so wouldn't need to go through context?
*/
func addContextModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("context").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expCurrentRound), nil, []api.ValueType{api.ValueTypeI64}).Export("current_round").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(cbor_parse_array), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("cbor_parse_array").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(create_obj_h), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("create_obj_h").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(create_obj_mem), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("create_obj_m").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expSerialize), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("serialize_obj").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expTxAttributes), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("tx_attributes").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expUnitData), []api.ValueType{api.ValueTypeI64, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("unit_data").
		Instantiate(ctx)
	return err
}

/*
create_obj_mem creates obj from data pointed by (shared) memory address.
Parameters (stack):
  - 0: type id (uint32)
  - 1: address (uint64)

Returns handle of the object.
*/
func create_obj_mem(vec *VmContext, mod api.Module, stack []uint64) error {
	// obj type, address of the data. version must be part of the raw (CBOR) data or we need param!?
	data := read(mod, stack[1])
	//vec.log.Debug(fmt.Sprintf("create_obj(%#v) => %#v", stack, data))
	typeID := api.DecodeU32(stack[0])
	obj, err := vec.factory.create_obj(typeID, data)
	if err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}
	stack[0] = vec.curPrg.AddVar(obj)
	return nil
}

/*
create_obj_h creates obj denoted by handle.
Parameters (stack):
  - 0: type id (uint32)
  - 1: handle (uint64)

Returns handle of the object.
*/
func create_obj_h(vec *VmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[1])
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	typeID := api.DecodeU32(stack[0])
	obj, err := vec.factory.create_obj(typeID, data)
	if err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}
	stack[0] = vec.curPrg.AddVar(obj)
	return nil
}

/*
cbor_parse_array, given handle of an raw CBOR buffer (stack[0]) parses it as array of raw CBOR items.
Returns list of (item) handles.
*/
func cbor_parse_array(vec *VmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[0])
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	var items []types.RawCBOR
	if err := cbor.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("decoding arguments as array of CBOR: %w", err)
	}

	// add items as new vars and return array of uint64 handles
	var buf encoder.WasmEnc
	buf.WriteUInt32(uint32(len(items)))
	for _, v := range items {
		buf.WriteUInt64(vec.curPrg.AddVar(v))
	}
	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for result: %w", err)
	}
	stack[0] = addr
	return nil
}

func expUnitData(vec *VmContext, mod api.Module, stack []uint64) error {
	id := read(mod, stack[0])
	vec.log.Debug(fmt.Sprintf("expUnitData(%#v) => %x", stack, id))
	unit, err := vec.curPrg.env.GetUnit(id, stack[1] != 0)
	if err != nil {
		return fmt.Errorf("reading unit data: %w", err)
	}
	data, err := vec.encoder.UnitData(unit, vec.curPrg.sdkVer)
	if err != nil {
		return fmt.Errorf("encoding unit data: %w", err)
	}
	addr, err := vec.writeToMemory(mod, data)
	if err != nil {
		return fmt.Errorf("allocating memory for predicate arguments: %w", err)
	}
	stack[0] = addr
	return nil
}

func expSerialize(vec *VmContext, mod api.Module, stack []uint64) error {
	v, ok := vec.curPrg.vars[stack[0]]
	if !ok {
		return fmt.Errorf("no variable with handle %d", stack[0])
	}
	data, err := vec.encoder.Encode(v, vec.curPrg.sdkVer, vec.curPrg.AddVar)
	if err != nil {
		return fmt.Errorf("encoding object: %w", err)
	}

	addr, err := vec.writeToMemory(mod, data)
	if err != nil {
		return fmt.Errorf("writing variable into shared memory: %w", err)
	}
	stack[0] = addr
	return nil
}

func expTxAttributes(vec *VmContext, mod api.Module, stack []uint64) error {
	vec.log.Info(fmt.Sprintf("expTxAttributes(%#v)", stack))

	txo, err := getVar[*types.TransactionOrder](vec.curPrg.vars, stack[0])
	if err != nil {
		return fmt.Errorf("reading tx order variable: %w", err)
	}
	buf, err := vec.encoder.TxAttributes(txo, vec.curPrg.sdkVer)
	if err != nil {
		return fmt.Errorf("encoding tx attributes: %w", err)
	}
	addr, err := vec.writeToMemory(mod, buf)
	if err != nil {
		return fmt.Errorf("allocating memory for tx attributes: %w", err)
	}
	stack[0] = addr
	return nil
}

func expCurrentRound(vec *VmContext, mod api.Module, stack []uint64) error {
	stack[0] = vec.curPrg.env.CurrentRound()
	return nil
}
