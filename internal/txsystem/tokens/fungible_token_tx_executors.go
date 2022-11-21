package tokens

import (
	"bytes"
	goerrors "errors"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
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

	joinFungibleTokenTxExecutor struct {
		*baseTxExecutor[*fungibleTokenTypeData]
		trustBase map[string]crypto.Verifier
	}
)

func (c *createFungibleTokenTypeTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*createFungibleTokenTypeWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Create Fungible Token Type tx: %v", tx)
	if err := c.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(c.hashAlgorithm)
	return c.state.AtomicUpdate(
		rma.AddItem(tx.UnitID(), script.PredicateAlwaysTrue(), newFungibleTokenTypeData(tx), h))
}

func (m *mintFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, _ uint64) error {
	tx, ok := gtx.(*mintFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Mint Fungible Token tx: %v", tx)
	if err := m.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(m.hashAlgorithm)
	return m.state.AtomicUpdate(
		rma.AddItem(tx.UnitID(), tx.attributes.Bearer, newFungibleTokenData(tx, m.hashAlgorithm), h))
}

func (t *transferFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*transferFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Transfer Fungible Token tx: %v", tx)
	if err := t.validate(tx); err != nil {
		return err
	}
	h := tx.Hash(t.hashAlgorithm)
	return t.state.AtomicUpdate(
		rma.SetOwner(tx.UnitID(), tx.attributes.NewBearer, h),
		rma.UpdateData(tx.UnitID(),
			func(data rma.UnitData) (newData rma.UnitData) {
				d, ok := data.(*fungibleTokenData)
				if !ok {
					return data
				}
				d.t = currentBlockNr
				d.backlink = tx.Hash(t.hashAlgorithm)
				return data
			}, h))
}

func (s *splitFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*splitFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Split Fungible Token tx: %v", tx)
	if err := s.validate(tx); err != nil {
		return err
	}
	u, err := s.state.GetUnit(tx.UnitID())
	if err != nil {
		return err
	}
	d := u.Data.(*fungibleTokenData)
	// add new token unit
	newTokenID := util.SameShardID(tx.UnitID(), tx.HashForIDCalculation(s.hashAlgorithm))
	logger.Debug("Adding a fungible token with ID %v", newTokenID)
	txHash := tx.Hash(s.hashAlgorithm)
	return s.state.AtomicUpdate(
		rma.AddItem(newTokenID,
			tx.attributes.NewBearer,
			&fungibleTokenData{
				tokenType: d.tokenType,
				value:     tx.attributes.TargetValue,
				t:         0,
				backlink:  make([]byte, s.hashAlgorithm.Size()),
			}, txHash),
		rma.UpdateData(tx.UnitID(),
			func(data rma.UnitData) (newData rma.UnitData) {
				d, ok := data.(*fungibleTokenData)
				if !ok {
					// No change in case of incorrect data type.
					return data
				}
				return &fungibleTokenData{
					tokenType: d.tokenType,
					value:     d.value - tx.attributes.TargetValue,
					t:         currentBlockNr,
					backlink:  txHash,
				}
			}, txHash))
}

func (b *burnFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*burnFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Burn Fungible Token tx: %v", tx)
	if err := b.validate(tx); err != nil {
		return err
	}
	unitID := tx.UnitID()
	h := tx.Hash(b.hashAlgorithm)
	return b.state.AtomicUpdate(
		rma.SetOwner(unitID, []byte{0}, h),
		rma.UpdateData(unitID,
			func(data rma.UnitData) rma.UnitData {
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
			}, h))
}

func (j *joinFungibleTokenTxExecutor) Execute(gtx txsystem.GenericTransaction, currentBlockNr uint64) error {
	tx, ok := gtx.(*joinFungibleTokenWrapper)
	if !ok {
		return errors.Errorf("invalid tx type: %T", gtx)
	}
	logger.Debug("Processing Join Fungible Token tx: %v", tx)
	if err := j.validate(tx); err != nil {
		return err
	}
	unitID := tx.UnitID()
	h := tx.Hash(j.hashAlgorithm)
	return j.state.AtomicUpdate(
		rma.UpdateData(unitID,
			func(data rma.UnitData) rma.UnitData {
				d, ok := data.(*fungibleTokenData)
				if !ok {
					// No change in case of incorrect data type.
					return data
				}
				var sum uint64 = 0
				for _, burnTransaction := range tx.burnTransactions {
					sum += burnTransaction.Value()
				}
				return &fungibleTokenData{
					tokenType: d.tokenType,
					value:     d.value + sum,
					t:         currentBlockNr,
					backlink:  h,
				}
			}, h))
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

	parentUnitID := tx.ParentTypeIdInt()
	if !parentUnitID.IsZero() {
		_, parentData, err := c.getUnit(parentUnitID)
		if err != nil {
			return err
		}
		if decimalPlaces != parentData.decimalPlaces {
			return errors.Errorf("invalid decimal places. allowed %v, got %v", parentData.decimalPlaces, decimalPlaces)
		}
	}
	predicates, err := c.getChainedPredicates(
		tx.ParentTypeIdInt(),
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
	return verifyPredicates(predicates, tx.SubTypeCreationPredicateSignatures(), tx)
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
	// existence of the parent type is checked by the getChainedPredicates
	predicates, err := m.getChainedPredicates(
		tx.TypeIDInt(),
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
	if len(predicates) > 0 {
		return script.RunScript(tx.attributes.TokenCreationPredicateSignature, predicates[0] /*TODO AB-478*/, tx.SigBytes())
	}
	return nil
}

func (t *transferFungibleTokenTxExecutor) validate(tx *transferFungibleTokenWrapper) error {
	d, err := t.getFungibleTokenData(tx.UnitID())
	if err != nil {
		return err
	}
	if d.value != tx.attributes.Value {
		return errors.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
	}

	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicates, err := t.getChainedPredicates(
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
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicates[0] /*TODO AB-479*/, tx.SigBytes())
}

func (s *splitFungibleTokenTxExecutor) validate(tx *splitFungibleTokenWrapper) error {
	d, err := s.getFungibleTokenData(tx.UnitID())
	if err != nil {
		return err
	}
	if d.value < tx.attributes.TargetValue {
		return errors.Errorf("invalid token value: max allowed %v, got %v", d.value, tx.attributes.TargetValue)
	}
	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicates, err := s.getChainedPredicates(
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
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicates[0] /*TODO AB-479*/, tx.SigBytes())
}

func (b *burnFungibleTokenTxExecutor) validate(tx *burnFungibleTokenWrapper) error {
	d, err := b.getFungibleTokenData(tx.UnitID())
	if err != nil {
		return err
	}
	tokenTypeID := d.tokenType.Bytes32()
	if !bytes.Equal(tokenTypeID[:], tx.attributes.Type) {
		return errors.Errorf("type of token to burn does not matches the actual type of the token: expected %X, got %X", tokenTypeID, tx.attributes.Type)
	}
	if tx.attributes.Value != d.value {
		return errors.Errorf("invalid token value: expected %v, got %v", d.value, tx.attributes.Value)
	}
	if !bytes.Equal(d.backlink, tx.attributes.Backlink) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.attributes.Backlink)
	}
	predicates, err := b.getChainedPredicates(
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
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicates[0] /*TODO AB-479*/, tx.SigBytes())
}

func (j *joinFungibleTokenTxExecutor) validate(tx *joinFungibleTokenWrapper) error {
	d, err := j.getFungibleTokenData(tx.UnitID())
	if err != nil {
		return err
	}
	transactions := tx.burnTransactions
	proofs := tx.BlockProofs()
	if len(transactions) != len(proofs) {
		return errors.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	for i, btx := range transactions {
		tokenTypeID := d.tokenType.Bytes32()
		if !bytes.Equal(btx.TypeID(), tokenTypeID[:]) {
			return errors.Errorf("the type of the burned source token does not match the type of target token: expected %X, got %X", tokenTypeID, btx.TypeID())
		}

		if !bytes.Equal(btx.Nonce(), tx.attributes.Backlink) {
			return errors.Errorf("the source tokens weren't burned to join them to the target token: source %X, target %X", btx.Nonce(), tx.Backlink())
		}
		proof := proofs[i]
		if proof.ProofType != block.ProofType_PRIM {
			return errors.New("invalid proof type")
		}

		err = proof.Verify(btx, j.trustBase, j.hashAlgorithm)
		if err != nil {
			return errors.Wrap(err, "proof is not valid")
		}
	}
	if !bytes.Equal(d.backlink, tx.Backlink()) {
		return errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.Backlink())
	}
	predicates, err := j.getChainedPredicates(
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
	return script.RunScript(tx.attributes.InvariantPredicateSignature, predicates[0] /*TODO AB-479*/, tx.SigBytes())
}
