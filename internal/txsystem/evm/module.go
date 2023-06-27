package evm

import (
	"crypto"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/ethereum/go-ethereum/core"
)

var _ txsystem.Module = &Module{}

type (
	Module struct {
		state            *rma.Tree
		systemIdentifier []byte
		trustBase        map[string]abcrypto.Verifier
		hashAlgorithm    crypto.Hash
		blockGasLimit    *core.GasPool
	}
)

func NewEVMModule(systemIdentifier []byte, options *Options) (*Module, error) {
	state := options.state

	if state == nil {
		return nil, fmt.Errorf("evm module init failed, state tree is nil")
	}

	return &Module{
		state:            state,
		systemIdentifier: systemIdentifier,
		trustBase:        options.trustBase,
		hashAlgorithm:    options.hashAlgorithm,
		blockGasLimit:    new(core.GasPool).AddGas(blockGasLimit),
	}, nil
}

func (m *Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		PayloadTypeEVMCall: handleEVMTx(m.state, m.hashAlgorithm, m.systemIdentifier, m.trustBase, m.blockGasLimit),
	}
}

func (m *Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
}

func (m *Module) BeginBlockFunc() []func(blockNr uint64) {
	return []func(blockNr uint64){
		func(blockNr uint64) {
			// reset block gas limit
			m.blockGasLimit = new(core.GasPool).AddGas(blockGasLimit)
		},
	}
}
