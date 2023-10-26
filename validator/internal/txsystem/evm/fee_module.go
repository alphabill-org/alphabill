package evm

import (
	"crypto"
	"fmt"
	"log/slog"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	abcrypto "github.com/alphabill-org/alphabill/validator/internal/crypto"
	"github.com/alphabill-org/alphabill/validator/internal/state"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/evm/statedb"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/fc/transactions"
)

var _ txsystem.Module = (*FeeAccount)(nil)

type (
	FeeAccount struct {
		state            *state.State
		systemIdentifier []byte
		trustBase        map[string]abcrypto.Verifier
		hashAlgorithm    crypto.Hash
		txValidator      *fc.DefaultFeeCreditTxValidator
		feeCalculator    FeeCalculator
		log              *slog.Logger
	}

	FeeCalculator func() uint64
)

func FixedFee(fee uint64) FeeCalculator {
	return func() uint64 {
		return fee
	}
}

func newFeeModule(systemIdentifier []byte, options *Options, log *slog.Logger) (*FeeAccount, error) {
	s := options.state
	if len(options.initialAccountAddress) > 0 && options.initialAccountBalance.Cmp(big.NewInt(0)) > 0 {
		address := common.BytesToAddress(options.initialAccountAddress)
		log.Info(fmt.Sprintf("Adding an initial account %v with balance %v", address, options.initialAccountBalance))
		id := s.Savepoint()
		stateDB := statedb.NewStateDB(s, log)
		stateDB.CreateAccount(address)
		stateDB.AddBalance(address, options.initialAccountBalance)
		s.ReleaseToSavepoint(id)
		_, _, err := s.CalculateRoot()
		if err != nil {
			return nil, err
		}
		err = s.Commit()
		if err != nil {
			return nil, err
		}
	}

	return &FeeAccount{
		state:            s,
		systemIdentifier: systemIdentifier,
		trustBase:        options.trustBase,
		hashAlgorithm:    options.hashAlgorithm,
		txValidator:      fc.NewDefaultFeeCreditTxValidator(options.moneyTXSystemIdentifier, systemIdentifier, options.hashAlgorithm, options.trustBase, nil),
		feeCalculator:    FixedFee(1),
		log:              log,
	}, nil
}

func (m FeeAccount) TxExecutors() map[string]txsystem.TxExecutor {
	return map[string]txsystem.TxExecutor{
		//  fee credit transaction handlers (credit transfers and reclaims only!)
		transactions.PayloadTypeAddFeeCredit:   addFeeCreditTx(m.state, m.hashAlgorithm, m.feeCalculator, m.txValidator),
		transactions.PayloadTypeCloseFeeCredit: closeFeeCreditTx(m.state, m.hashAlgorithm, m.feeCalculator, m.txValidator, m.log),
	}
}

func (m FeeAccount) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return checkFeeAccountBalance(m.state)
}
