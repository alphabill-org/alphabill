package vd

import (
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
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
		txExecutors []txsystem.TxExecutor
		txConverter txsystem.TxConverters
	}

	unit struct {
		dataHash    []byte
		blockNumber uint64
	}
)

func NewVDModule(options *Options) (*VDModule, error) {
	return &VDModule{
		txExecutors: []txsystem.TxExecutor{
			handleRegisterDataTx(options),
		},
		txConverter: map[string]txsystem.TxConverter{
			TypeURLRegisterDataAttributes: convertRegisterData,
		},
	}, nil
}

func (n *VDModule) TxExecutors() []txsystem.TxExecutor {
	return n.txExecutors
}

func (n *VDModule) GenericTransactionValidator() txsystem.GenericTransactionValidator {
	return func(ctx *txsystem.TxValidationContext) error {
		typeUrl := ctx.Tx.ToProtoBuf().TransactionAttributes.TypeUrl
		if typeUrl != TypeURLRegisterDataAttributes {
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

func (n *VDModule) TxConverter() txsystem.TxConverters {
	return n.txConverter
}

func handleRegisterDataTx(options *Options) txsystem.GenericExecuteFunc[*registerDataWrapper] {
	return func(tx *registerDataWrapper,  currentBlockNumber uint64) error {
		log.Debug("Processing register data tx: '%v', UnitID=%x", tx, tx.UnitID())

		if len(tx.OwnerProof()) > 0 {
			return ErrOwnerProofPresent
		}

		fee := options.feeCalculator()
		tx.SetServerMetadata(&txsystem.ServerMetadata{Fee: fee})
		txHash := tx.Hash(options.hashAlgorithm)

		fcrID := tx.transaction.GetClientFeeCreditRecordID()
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, txHash),
			rma.AddItem(tx.UnitID(),
				script.PredicateAlwaysFalse(),
				&unit{
					dataHash:    abhash.Sum256(util.Uint256ToBytes(tx.UnitID())),
					blockNumber: currentBlockNumber,
				},
				txHash,
			),
		)
	}
}

func (u *unit) AddToHasher(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) Value() rma.SummaryValue {
	return zeroSummaryValue
}
