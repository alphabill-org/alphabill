package tokens

import (
	"bytes"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/util"
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

	burnFungibleTokenTxExecutor struct {
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

func (s *splitFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*splitFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := s.validate(tx); err != nil {
		return err
	}
	u, err := s.state.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	d := u.Data.(*fungibleTokenData)
	// add new token unit
	newTokenID := util.SameShardId(tx.UnitID(), tx.HashForIdCalculation(s.hashAlgorithm))
	logger.Debug("Adding a fungible token with ID %v", newTokenID)
	txHash := tx.Hash(s.hashAlgorithm)
	err = s.state.AddItem(newTokenID, tx.attributes.NewBearer, &fungibleTokenData{
		tokenType: d.tokenType,
		value:     tx.attributes.Value,
		t:         0,
		backlink:  make([]byte, s.hashAlgorithm.Size()),
	}, txHash)
	if err != nil {
		return err
	}
	return s.state.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
		d, ok := data.(*fungibleTokenData)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		return &fungibleTokenData{
			tokenType: d.tokenType,
			value:     d.value - tx.attributes.Value,
			t:         currentBlockNr,
			backlink:  txHash,
		}
	}, txHash)
}

func (b *burnFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*burnFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := b.validate(tx); err != nil {
		return err
	}
	unitID := tx.UnitID()
	h := tx.Hash(b.hashAlgorithm)
	if err := b.state.SetOwner(unitID, []byte{0}, h); err != nil {
		return err
	}
	return b.state.UpdateData(unitID, func(data rma.UnitData) rma.UnitData {
		d, ok := data.(*fungibleTokenData)
		if !ok {
			// No change in case of incorrect data type.
			return data
		}
		return &fungibleTokenData{
			tokenType: d.tokenType,
			value:     d.value,
			t:         currentBlockNr,
			backlink:  h,
		}
	}, h)
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
	// existence of the parent type is checked by the getChainedPredicate
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

func (s *splitFungibleTokenTxExecutor) validate(tx *splitFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := s.state.GetUnit(unitID)
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
	if d.value < tx.attributes.Value {
		return errors.Errorf("invalid token value: max allowed %v, got %v", d.value, tx.attributes.Value)
	}

	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicate, err := s.getChainedPredicate(
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

func (b *burnFungibleTokenTxExecutor) validate(tx *burnFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := b.state.GetUnit(unitID)
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

	if !bytes.Equal(d.tokenType.Bytes(), tx.attributes.Type) {
		return errors.Errorf("type of token to burn does not matches the actual type of the token: expected %X, got %X", d.tokenType.Bytes(), tx.attributes.Type)
	}

	if tx.attributes.Value != d.value {
		return errors.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
	}
	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicate, err := b.getChainedPredicate(
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
