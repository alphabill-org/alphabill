package evm

import (
	"crypto"
	"math/big"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/statedb"
	"github.com/ethereum/go-ethereum/common"
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
	if len(options.initialAccountAddress) > 0 && options.initialAccountBalance.Cmp(big.NewInt(0)) > 0 {
		address := common.BytesToAddress(options.initialAccountAddress)
		log.Info("Adding an initial account %v with balance %v", address, options.initialAccountBalance)
		stateDB := statedb.NewStateDB(state)
		stateDB.CreateAccount(address)
		stateDB.AddBalance(address, options.initialAccountBalance)
		state.Commit()
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
