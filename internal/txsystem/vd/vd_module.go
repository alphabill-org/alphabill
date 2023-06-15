package vd

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	zeroSummaryValue = rma.Uint64SummaryValue(0)
)

var (
	ErrOwnerProofPresent = errors.New("'register data' transaction cannot have an owner proof")
	ErrDataAlreadyExists = errors.New("data already exists")
	log                  = logger.CreateForPackage()

)

type (
	VDModule struct {
		txExecutors map[string]txsystem.TxExecutor
	}

	unit struct {
		dataHash    []byte
		blockNumber uint64
	}
)

func NewVDModule(options *Options) (*VDModule, error) {
	return &VDModule{
		txExecutors: map[string]txsystem.TxExecutor{
			PayloadTypeRegisterData: handleRegisterDataTx(options),
		},
	}, nil
}

func (n *VDModule) TxExecutors() map[string]txsystem.TxExecutor {
	return n.txExecutors
}

func (n *VDModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		if ctx.Tx.PayloadType() != PayloadTypeRegisterData {
			return txsystem.ValidateGenericTransaction(ctx)
		}

		err := txsystem.ValidateGenericTransaction(&txsystem.TxValidationContext{
			Tx:               ctx.Tx,
			Unit:             nil, // VD transactions do not have owner proofs
			SystemIdentifier: ctx.SystemIdentifier,
			BlockNumber:      ctx.BlockNumber,
		})
		if err != nil {
			return err
		}
		if ctx.Unit != nil {
			return ErrDataAlreadyExists
		}
		return nil
	}
}

func handleRegisterDataTx(options *Options) txsystem.GenericExecuteFunc[RegisterDataAttributes] {
	return func(tx *types.TransactionOrder, attr *RegisterDataAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		log.Debug("Processing register data tx: '%v', UnitID=%x", tx, tx.UnitID())

		if len(tx.OwnerProof) > 0 {
			return nil, ErrOwnerProofPresent
		}

		fee := options.feeCalculator()
		txHash := tx.Hash(options.hashAlgorithm)

		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, txHash),
			rma.AddItem(unitID,
				script.PredicateAlwaysFalse(),
				&unit{
					dataHash:    abhash.Sum256(tx.UnitID()),
					blockNumber: currentBlockNr,
				},
				txHash,
			),
		)
		if err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func (u *unit) AddToHasher(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) Value() rma.SummaryValue {
	return zeroSummaryValue
}
