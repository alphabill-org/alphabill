package tokens

import (
	"bytes"
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

func (m *mintFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*mintFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := m.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(m.hashAlgorithm)
	return m.state.AddItem(
		tx.UnitID(),
		tx.attributes.Bearer,
		newFungibleTokenData(tx, m.hashAlgorithm),
		h,
	)
	return nil
}

func (t *transferFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*transferFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := t.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(t.hashAlgorithm)
	if err := t.state.SetOwner(tx.UnitID(), tx.attributes.NewBearer, h); err != nil {
		return err
	}
	return t.state.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
		d, ok := data.(*fungibleTokenData)
		if !ok {
			return data
		}
		d.t = currentBlockNr
		d.backlink = tx.Hash(t.hashAlgorithm)
		return data
	}, h)
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

func (m *mintFungibleTokenTxExecutor) validate(tx *mintFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := m.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	predicate, err := m.getChainedPredicate(
		tx.TypeID(),
		func(d *fungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.TokenCreationPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}

func (t *transferFungibleTokenTxExecutor) validate(tx *transferFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := t.state.GetUnit(unitID)
	if err != nil {
		if goerrors.Is(err, rma.ErrUnitNotFound) {
			return errors.Wrapf(err, "unit %v does not exist", unitID)
		}
		return err
	}

	d, ok := u.Data.(*fungibleTokenData)
	if !ok {
		return errors.Errorf("unit %v is not fungible token data", unitID)
	}
	if d.value != tx.attributes.Value {
		return errors.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
	}

	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicate, err := t.getChainedPredicate(
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	if len(predicate) > 0 {
		return script.RunScript(tx.attributes.InvariantPredicateSignature, predicate, tx.SigBytes())
	}
	return nil
}
