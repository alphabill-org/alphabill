package program

import (
	"context"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
)

type SmartContractModule struct {
	ctx              context.Context
	state            *rma.Tree
	systemIdentifier []byte
	hashAlgorithm    gocrypto.Hash
}

func NewSmartContractModule(ctx context.Context, systemIdentifier []byte, options *Options) (txsystem.Module, error) {
	return &SmartContractModule{
		ctx:              ctx,
		systemIdentifier: systemIdentifier,
		hashAlgorithm:    options.hashAlgorithm,
		state:            options.state,
	}, nil
}

func (s *SmartContractModule) TxExecutors() []txsystem.TxExecutor {
	return []txsystem.TxExecutor{
		handlePDeployTx(s.ctx, s.state, s.systemIdentifier, s.hashAlgorithm),
		handlePCallTx(s.ctx, s.state, s.systemIdentifier, s.hashAlgorithm),
	}
}

func (s *SmartContractModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		return txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{
			Tx:               ctx.Tx,
			Unit:             nil, // program transactions do not have owner proofs.
			SystemIdentifier: ctx.SystemIdentifier,
			BlockNumber:      ctx.BlockNumber,
		})
	}

}

func (s *SmartContractModule) TxConverter() txsystem.TxConverters {
	return txsystem.TxConverters{
		typeURLPCall:   convertPCallTx(),
		typeURLPDeploy: convertPDeployTx(),
	}
}

func convertPCallTx() func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
		attr := &PCallAttributes{}
		if err := tx.TransactionAttributes.UnmarshalTo(attr); err != nil {
			return nil, fmt.Errorf("invalid tx attributes: %w", err)
		}
		return &PCallTransactionOrder{attributes: attr, txOrder: tx}, nil
	}
}

func convertPDeployTx() func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return func(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
		attr := &PDeployAttributes{}
		if err := tx.TransactionAttributes.UnmarshalTo(attr); err != nil {
			return nil, fmt.Errorf("invalid tx attributes: %w", err)
		}
		return &PDeployTransactionOrder{attributes: attr, txOrder: tx}, nil
	}
}
