package sc

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type SmartContractModule struct {
	state            *rma.Tree
	systemIdentifier []byte
	hashAlgorithm    gocrypto.Hash
	programs         BuiltInPrograms
}

func NewSmartContractModule(systemIdentifier []byte, options *Options) (txsystem.Module, error) {
	programs, err := initBuiltInPrograms(options.state)
	if err != nil {
		return nil, fmt.Errorf("failed to init built-in programs: %w", err)
	}
	return &SmartContractModule{
		systemIdentifier: systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            options.state,
		programs:         programs,
	}, nil
}

func (s *SmartContractModule) TxExecutors() []txsystem.TxExecutor {
	return []txsystem.TxExecutor{handleSCallTx(s.state, s.programs, s.systemIdentifier, s.hashAlgorithm)}
}

func (s *SmartContractModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		return txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{
			Tx:               ctx.Tx,
			Unit:             nil, // SC transactions do not have owner proofs.
			SystemIdentifier: ctx.SystemIdentifier,
			BlockNumber:      ctx.BlockNumber,
		})
	}

}

func (s *SmartContractModule) TxConverter() txsystem.TxConverters {
	return txsystem.TxConverters{
		typeURLSCall: convertSCallTx(),
	}
}

func convertSCallTx() func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
		attr := &SCallAttributes{}
		if err := tx.TransactionAttributes.UnmarshalTo(attr); err != nil {
			return nil, fmt.Errorf("invalid tx attributes: %w", err)
		}
		return &SCallTransactionOrder{attributes: attr, txOrder: tx}, nil
	}
}
