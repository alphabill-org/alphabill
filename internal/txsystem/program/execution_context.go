package program

import (
	"crypto"

	"github.com/holiman/uint256"
)

type ExecutionContext struct {
	programID     *uint256.Int
	inputParams   []byte
	progParams    []byte
	hashAlgorithm crypto.Hash
	txHash        []byte
}

func NewExecCtxFormPCall(txOrder *PCallTransactionOrder, params []byte, hash crypto.Hash) (*ExecutionContext, error) {
	return &ExecutionContext{
		programID:     txOrder.UnitID(),
		inputParams:   txOrder.attributes.Input,
		progParams:    params,
		hashAlgorithm: hash,
		txHash:        txOrder.Hash(hash),
	}, nil
}

func NewExecCtxFormDeploy(txOrder *PDeployTransactionOrder, hash crypto.Hash) (*ExecutionContext, error) {

	return &ExecutionContext{
		hashAlgorithm: hash,
		programID:     txOrder.UnitID(),
		inputParams:   txOrder.attributes.InitData,
		progParams:    txOrder.attributes.InitData,
		txHash:        txOrder.Hash(hash),
	}, nil
}

func (e ExecutionContext) GetProgramID() *uint256.Int {
	return e.programID
}

func (e ExecutionContext) GetInputData() []byte {
	return e.inputParams
}

func (e ExecutionContext) GetParams() []byte {
	return e.progParams
}

func (e ExecutionContext) GetTxHash() []byte {
	return e.txHash
}
