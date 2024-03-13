package wvm

import (
	"context"
	"crypto"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/types"
)

/*
AB functions to verify objects etc
*/
func addAlphabillModule(ctx context.Context, rt wazero.Runtime, observe Observability) error {
	_, err := rt.NewHostModuleBuilder("ab").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(digest_sha256), []api.ValueType{api.ValueTypeI64}, []api.ValueType{api.ValueTypeI64}).Export("digest_sha256").
		NewFunctionBuilder().WithGoModuleFunction(hostAPI(verifyTxProof), []api.ValueType{api.ValueTypeI64, api.ValueTypeI64}, []api.ValueType{api.ValueTypeI32}).Export("verify_tx_proof").
		Instantiate(ctx)
	return err
}

func verifyTxProof(vec *VmContext, mod api.Module, stack []uint64) error {
	// args: handle of txProof, handle of txRec
	proof, err := getVar[*types.TxProof](vec.curPrg.vars, stack[0])
	if err != nil {
		return fmt.Errorf("tx proof: %w", err)
	}
	txRec, err := getVar[*types.TransactionRecord](vec.curPrg.vars, stack[1])
	if err != nil {
		return fmt.Errorf("tx record: %w", err)
	}
	tb, err := vec.curPrg.env.TrustBase()
	if err != nil {
		return fmt.Errorf("acquiring trust base: %w", err)
	}
	if err := types.VerifyTxProof(proof, txRec, tb, crypto.SHA256); err != nil {
		vec.log.Debug(fmt.Sprintf("%s.verifyTxProof: %v", mod.Name(), err))
		stack[0] = 1
	} else {
		stack[0] = 0
	}
	return nil
}

func digest_sha256(vec *VmContext, mod api.Module, stack []uint64) error {
	data := read(mod, stack[0])
	//vec.log.Debug(fmt.Sprintf("digest_sha256(%#v) => %#v", stack, data))

	addr, err := vec.writeToMemory(mod, hash.Sum256(data))
	if err != nil {
		return fmt.Errorf("allocating memory for tx attributes: %w", err)
	}
	stack[0] = addr
	return nil
}
