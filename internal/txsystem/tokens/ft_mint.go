package tokens

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
)

func handleMintFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[MintFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Mint Fungible Token tx: %v", tx)
		if err := validateMintFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid mint fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.AddItem(unitID, attr.Bearer, newFungibleTokenData(attr, h, currentBlockNr), h),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateMintFungibleToken(tx *types.TransactionOrder, attr *MintFungibleTokenAttributes, state *rma.Tree) error {
	unitID := util.BytesToUint256(tx.UnitID())
	if unitID.IsZero() {
		return errors.New(ErrStrUnitIDIsZero)
	}
	u, err := state.GetUnit(unitID)
	if u != nil {
		return fmt.Errorf("unit with id %v already exists", unitID)
	}
	if !errors.Is(err, rma.ErrUnitNotFound) {
		return err
	}
	if attr.Value == 0 {
		return errors.New("token must have value greater than zero")
	}

	// existence of the parent type is checked by the getChainedPredicates
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		state,
		util.BytesToUint256(attr.TypeID),
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
	sigBytes, err := tx.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.TokenCreationPredicateSignatures, sigBytes)
}

func (m *MintFungibleTokenAttributes) GetBearer() []byte {
	return m.Bearer
}

func (m *MintFungibleTokenAttributes) SetBearer(bearer []byte) {
	m.Bearer = bearer
}

func (m *MintFungibleTokenAttributes) GetTypeID() []byte {
	return m.TypeID
}

func (m *MintFungibleTokenAttributes) SetTypeID(typeID []byte) {
	m.TypeID = typeID
}

func (m *MintFungibleTokenAttributes) GetValue() uint64 {
	return m.Value
}

func (m *MintFungibleTokenAttributes) SetValue(value uint64) {
	m.Value = value
}

func (m *MintFungibleTokenAttributes) GetTokenCreationPredicateSignatures() [][]byte {
	return m.TokenCreationPredicateSignatures
}

func (m *MintFungibleTokenAttributes) SetTokenCreationPredicateSignatures(signatures [][]byte) {
	m.TokenCreationPredicateSignatures = signatures
}

func (m *MintFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016
	signatureAttr := &MintFungibleTokenAttributes{
		Bearer:                           m.Bearer,
		TypeID:                           m.TypeID,
		Value:                            m.Value,
		TokenCreationPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
