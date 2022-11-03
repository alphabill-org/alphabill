package tokens

import (
	"bytes"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type (
	createNonFungibleTokenTypeTxExecutor struct {
		*baseTxExecutor[*nonFungibleTokenTypeData]
	}

	mintNonFungibleTokenTxExecutor struct {
		*baseTxExecutor[*nonFungibleTokenTypeData]
	}

	transferNonFungibleTokenTxExecutor struct {
		*baseTxExecutor[*nonFungibleTokenTypeData]
	}

	updateNonFungibleTokenTxExecutor struct {
		*baseTxExecutor[*nonFungibleTokenTypeData]
	}
)

func (c *createNonFungibleTokenTypeTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*createNonFungibleTokenTypeWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := c.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(c.hashAlgorithm)
	return c.state.AtomicUpdate(
		rma.AddItem(tx.UnitID(), script.PredicateAlwaysTrue(), newNonFungibleTokenTypeData(tx), h))
}

func (m *mintNonFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*mintNonFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := m.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(m.hashAlgorithm)
	return m.state.AtomicUpdate(
		rma.AddItem(tx.UnitID(), tx.attributes.Bearer, newNonFungibleTokenData(tx, m.hashAlgorithm), h))
}

func (t *transferNonFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*transferNonFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := t.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(t.hashAlgorithm)
	return t.state.AtomicUpdate(
		rma.SetOwner(tx.UnitID(), tx.attributes.NewBearer, h),
		rma.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
			d, ok := data.(*nonFungibleTokenData)
			if !ok {
				return data
			}
			d.t = currentBlockNr
			d.backlink = tx.Hash(t.hashAlgorithm)
			return data
		}, h))
}

func (te *updateNonFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*updateNonFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	if err := te.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(te.hashAlgorithm)
	return te.state.AtomicUpdate(
		rma.UpdateData(tx.UnitID(), func(data rma.UnitData) (newData rma.UnitData) {
			d, ok := data.(*nonFungibleTokenData)
			if !ok {
				return data
			}
			d.data = tx.attributes.Data
			d.t = currentBlockNr
			d.backlink = tx.Hash(te.hashAlgorithm)
			return data
		}, h))
}

func (c *createNonFungibleTokenTypeTxExecutor) validate(tx *createNonFungibleTokenTypeWrapper) error {
	unitID := tx.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	if len(tx.attributes.Symbol) > maxSymbolLength {
		return errors.Errorf(ErrStrInvalidSymbolName)
	}
	u, err := c.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	// signature satisfies the predicate obtained by concatenating all the
	// sub-type creation clauses along the type inheritance chain.
	predicate, err := c.getChainedPredicate(
		tx.parentTypeIdInt(),
		func(d *nonFungibleTokenTypeData) []byte {
			return d.subTypeCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
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

func (m *mintNonFungibleTokenTxExecutor) validate(tx *mintNonFungibleTokenWrapper) error {
	unitID := tx.wrapper.UnitID()
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	uri := tx.URI()
	if uri != "" {
		if len(uri) > uriMaxSize {
			return errors.Errorf("URI exceeds the maximum allowed size of %v KB", uriMaxSize)
		}
		if !util.IsValidURI(uri) {
			return errors.Errorf("URI %s is invalid", uri)
		}
	}
	if len(tx.Data()) > dataMaxSize {
		return errors.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	u, err := m.state.GetUnit(unitID)
	if u != nil {
		return errors.Errorf("unit %v exists", unitID)
	}
	if !goerrors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	nftTypeID := tx.NFTTypeIdInt()
	if nftTypeID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}

	// the transaction request satisfies the predicate obtained by concatenating all the token creation clauses along
	// the type inheritance chain.
	predicate, err := m.getChainedPredicate(
		nftTypeID,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.tokenCreationPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
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

func (t *transferNonFungibleTokenTxExecutor) validate(tx *transferNonFungibleTokenWrapper) error {
	unitID := tx.UnitID()
	u, err := t.state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return errors.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, tx.attributes.Backlink) {
		return errors.New("invalid backlink")
	}
	// signature given in the transaction request satisfies the predicate obtained by concatenating all the token
	// invariant clauses along the type inheritance chain.
	predicate, err := t.getChainedPredicate(
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicate, tx.SigBytes())
}

func (te *updateNonFungibleTokenTxExecutor) validate(tx *updateNonFungibleTokenWrapper) error {
	if len(tx.attributes.Data) > dataMaxSize {
		return errors.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := tx.UnitID()
	u, err := te.state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return errors.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, tx.attributes.Backlink) {
		return errors.New("invalid backlink")
	}
	predicate, err := te.getChainedPredicate(
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.dataUpdatePredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	predicate = append(predicate, data.dataUpdatePredicate...)
	if err != nil {
		return err
	}
	return script.RunScript(tx.attributes.DataUpdateSignature, predicate, tx.SigBytes())
}
