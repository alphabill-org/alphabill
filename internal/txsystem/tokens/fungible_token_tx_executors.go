package tokens

import (
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
)

type (
	createFungibleTokenTypeTxExecutor struct {
		*baseTxExecutor[*fungibleTokenTypeData]
	}

	mintFungibleTokenTxExecutor struct {
		*baseTxExecutor[*fungibleTokenTypeData]
	}

	transferFungibleTokenTxExecutor struct {
		*baseTxExecutor[*fungibleTokenTypeData]
	}

	splitFungibleTokenTxExecutor struct {
		*baseTxExecutor[*fungibleTokenTypeData]
	}
)

func (c *createFungibleTokenTypeTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*createFungibleTokenTypeWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := c.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(c.hashAlgorithm)
	return c.state.AddItem(
		tx.UnitID(),
		script.PredicateAlwaysTrue(),
		newFungibleTokenTypeData(tx),
		h,
	)
	return nil
}

func (m *mintFungibleTokenTxExecutor) Execute(tx txsystem.GenericTransaction, currentBlockNr uint64) error {
	//TODO AB-345
	panic("implement me")
}

func (t *transferFungibleTokenTxExecutor) Execute(tx txsystem.GenericTransaction, currentBlockNr uint64) error {
	//TODO AB-346
	panic("implement me")
}

func (s *splitFungibleTokenTxExecutor) Execute(tx txsystem.GenericTransaction, currentBlockNr uint64) error {
	//TODO AB-350
	panic("implement me")
}

func (c *createFungibleTokenTypeTxExecutor) validate(tx *createFungibleTokenTypeWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	if len(tx.attributes.Symbol) > maxSymbolLength {
		return errors.New(ErrStrInvalidSymbolName)
	}
	decimalPlaces := tx.attributes.DecimalPlaces
	if decimalPlaces > maxDecimalPlaces {
		return errors.Errorf("invalid decimal places. maximum allowed value %v, got %v", maxDecimalPlaces, decimalPlaces)
	}

	u, err := c.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}

	parentUnitID := tx.ParentTypeID()
	if !parentUnitID.IsZero() {
		_, parentData, err := c.getUnit(parentUnitID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.decimalPlaces {
			return errors.Errorf("invalid decimal places. allowed %v, got %v", parentData.decimalPlaces, decimalPlaces)
		}
	}
	predicate, err := c.getChainedPredicate(
		tx.ParentTypeID(),
		func(d *fungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.SubTypeCreationPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}
