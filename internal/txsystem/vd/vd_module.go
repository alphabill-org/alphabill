package vd

import (
	"bytes"
	"hash"

	"github.com/alphabill-org/alphabill/internal/state"

	"github.com/alphabill-org/alphabill/internal/errors"
	abhash "github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/logger"
)

const (
	zeroSummaryValue = uint64(0)
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
		unitID := tx.UnitID()
		err := options.state.Apply(
			state.AddUnit(unitID,
				script.PredicateAlwaysFalse(),
				&unit{
					dataHash:    abhash.Sum256(tx.UnitID()),
					blockNumber: currentBlockNr,
				},
			),
		)
		if err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{tx.UnitID()}}, nil
	}
}

func (u *unit) Write(hasher hash.Hash) {
	hasher.Write(u.dataHash)
	hasher.Write(util.Uint64ToBytes(u.blockNumber))
}

func (u *unit) SummaryValueInput() uint64 {
	return zeroSummaryValue
}

func (u *unit) Copy() state.UnitData {
	if u == nil {
		return u
	}
	return &unit{
		dataHash:    bytes.Clone(u.dataHash),
		blockNumber: u.blockNumber,
	}
}
