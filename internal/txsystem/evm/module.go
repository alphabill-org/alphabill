package evm

import (
	"crypto"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

var _ txsystem.Module = &Module{}

type (
	Module struct {
		state            *rma.Tree
		systemIdentifier []byte
		trustBase        map[string]abcrypto.Verifier
		hashAlgorithm    crypto.Hash
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
	}, nil
}

func (m Module) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		PayloadTypeEVMCall: handleEVMTx(m.state, m.hashAlgorithm, m.systemIdentifier, m.trustBase),
	}
}

func (m Module) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return txsystem.ValidateGenericTransaction
}
