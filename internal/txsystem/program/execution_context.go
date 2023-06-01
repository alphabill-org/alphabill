package program

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type ExecutionContext struct {
	programID     *uint256.Int
	inputParams   []byte
	progParams    []byte
	hashAlgorithm crypto.Hash
	txHash        []byte
}

func NewExecCtxFormPCall(tx *types.TransactionOrder, attr *PCallAttributes, params []byte, hash crypto.Hash) (*ExecutionContext, error) {
	return &ExecutionContext{
		programID:     util.BytesToUint256(tx.UnitID()),
		inputParams:   attr.InputData,
		progParams:    params,
		hashAlgorithm: hash,
		txHash:        tx.Hash(hash),
	}, nil
}

func NewExecCtxFormDeploy(tx *types.TransactionOrder, attr *PDeployAttributes, hash crypto.Hash) (*ExecutionContext, error) {
	return &ExecutionContext{
		hashAlgorithm: hash,
		programID:     util.BytesToUint256(tx.UnitID()),
		inputParams:   []byte{},
		progParams:    attr.ProgParams,
		txHash:        tx.Hash(hash),
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
