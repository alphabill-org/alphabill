package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

func handleUpdateNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[UpdateNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		if err := validateUpdateNonFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid update non-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if err := options.state.Apply(
			state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*NonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.Data = attr.Data
				d.T = currentBlockNr
				d.Backlink = tx.Hash(options.hashAlgorithm)
				return d, nil
			})); err != nil {
			return nil, err
		}

		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateUpdateNonFungibleToken(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := tx.UnitID()
	if !unitID.HasType(NonFungibleTokenUnitType) {
		return fmt.Errorf(ErrStrInvalidUnitID)
	}
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*NonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if data.Locked != 0 {
		return errors.New("token is locked")
	}
	if !bytes.Equal(data.Backlink, attr.Backlink) {
		return errors.New("invalid backlink")
	}
	ps, err := getChainedPredicates[*NonFungibleTokenTypeData](
		hashAlgorithm,
		s,
		data.NftType,
		func(d *NonFungibleTokenTypeData) []byte {
			return d.DataUpdatePredicate
		},
		func(d *NonFungibleTokenTypeData) types.UnitID {
			return d.ParentTypeId
		},
	)
	if err != nil {
		return err
	}
	ps = append([]predicates.PredicateBytes{data.DataUpdatePredicate}, ps...)
	sigBytes, err := tx.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return err
	}
	return verifyPredicates(ps, attr.DataUpdateSignatures, sigBytes)
}

func (u *UpdateNonFungibleTokenAttributes) GetData() []byte {
	return u.Data
}

func (u *UpdateNonFungibleTokenAttributes) SetData(data []byte) {
	u.Data = data
}

func (u *UpdateNonFungibleTokenAttributes) GetBacklink() []byte {
	return u.Backlink
}

func (u *UpdateNonFungibleTokenAttributes) SetBacklink(backlink []byte) {
	u.Backlink = backlink
}

func (u *UpdateNonFungibleTokenAttributes) GetDataUpdateSignatures() [][]byte {
	return u.DataUpdateSignatures
}

func (u *UpdateNonFungibleTokenAttributes) SetDataUpdateSignatures(signatures [][]byte) {
	u.DataUpdateSignatures = signatures
}

func (u *UpdateNonFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude DataUpdateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UpdateNonFungibleTokenAttributes{
		Data:                 u.Data,
		Backlink:             u.Backlink,
		DataUpdateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}
