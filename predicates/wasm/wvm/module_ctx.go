package wvm

import (
	"context"
	"errors"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill-go-base/types"
)

/*
addContextModule adds "context" module to the "rt".
This module provides access to "current predicate evaluation context",
ie current transaction, predicate arguments, round number etc.
*/
func addContextModule(ctx context.Context, rt wazero.Runtime, _ Observability) error {
	_, err := rt.NewHostModuleBuilder("context").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expCurrentTime), nil, []api.ValueType{api.ValueTypeI64}).Export("now").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expCurrentRound), nil, []api.ValueType{api.ValueTypeI64}).Export("current_round").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(createObjH), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("create_obj_h").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(createObjMem), []api.ValueType{api.ValueTypeI32, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("create_obj_m").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expSerialize), []api.ValueType{api.ValueTypeI64, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("serialize_obj").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expTxAttributes), []api.ValueType{api.ValueTypeI64, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("tx_attributes").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(expUnitData), []api.ValueType{api.ValueTypeI64, api.ValueTypeI32, api.ValueTypeI32}, []api.ValueType{api.ValueTypeI64}).Export("unit_data").
		Instantiate(ctx)
	return err
}

/*
createObjMem creates obj from data pointed by (shared) memory address.
Parameters (stack):
  - 0: type id (uint32)
  - 1: address (uint64)

Returns handle of the object.
*/
func createObjMem(vec *vmContext, mod api.Module, stack []uint64) error {
	// obj type, address of the data. version must be part of the raw (CBOR) data or we need param!?
	data := read(mod, stack[1])
	typeID := api.DecodeU32(stack[0])
	obj, err := vec.factory.createObj(typeID, data)
	if err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}
	stack[0] = vec.curPrg.addVar(obj)
	return nil
}

/*
createObjH creates obj denoted by handle.
Parameters (stack):
  - 0: type id (uint32)
  - 1: handle (uint64)

Returns handle of the object.
*/
func createObjH(vec *vmContext, mod api.Module, stack []uint64) error {
	data, err := vec.getBytesVariable(stack[1])
	if err != nil {
		return fmt.Errorf("reading variable: %w", err)
	}

	typeID := api.DecodeU32(stack[0])
	obj, err := vec.factory.createObj(typeID, data)
	if err != nil {
		return fmt.Errorf("decoding object: %w", err)
	}
	stack[0] = vec.curPrg.addVar(obj)
	return nil
}

func expUnitData(vec *vmContext, mod api.Module, stack []uint64) error {
	id := read(mod, stack[0])
	unit, err := vec.curPrg.env.GetUnit(id, stack[1] != 0)
	if err != nil {
		return fmt.Errorf("reading unit data: %w", err)
	}
	data, err := vec.encoder.UnitData(unit, api.DecodeU32(stack[2]))
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

/*
Arguments in stack:
  - 0: handle of the variable;
  - 1: version of the data struct (in the SDK);
*/
func expSerialize(vec *vmContext, mod api.Module, stack []uint64) error {
	v, ok := vec.curPrg.vars[stack[0]]
	if !ok {
		return fmt.Errorf("no variable with handle %d", stack[0])
	}
	data, err := vec.encoder.Encode(v, api.DecodeU32(stack[1]), vec.curPrg.addVar)
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

func expTxAttributes(vec *vmContext, mod api.Module, stack []uint64) error {
	txo, err := getVar[*types.TransactionOrder](vec.curPrg.vars, stack[0])
	if err != nil {
		return fmt.Errorf("reading tx order variable: %w", err)
	}
	buf, err := vec.encoder.TxAttributes(txo, api.DecodeU32(stack[1]))
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

func expCurrentRound(vec *vmContext, mod api.Module, stack []uint64) error {
	stack[0] = vec.curPrg.env.CurrentRound()
	return nil
}

func expCurrentTime(vec *vmContext, mod api.Module, stack []uint64) error {
	uc := vec.curPrg.env.CommittedUC()
	if uc == nil {
		return errors.New("no committed UC available")
	}
	stack[0] = uc.UnicitySeal.Timestamp
	return nil
}
