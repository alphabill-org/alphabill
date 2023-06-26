package tokens

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/state"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
)

func handleUpdateNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[UpdateNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Update Non-Fungible Token tx: %v", tx)
		if err := validateUpdateNonFungibleToken(tx, attr, options.state, options.hashAlgorithm); err != nil {
			return nil, fmt.Errorf("invalid update none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		unitID := tx.UnitID()
		sm := &types.ServerMetadata{ActualFee: fee}
		txr := &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   sm,
		}

		// update state
		if err := options.state.Apply(
			state.UpdateUnitData(unitID, func(data state.UnitData) (state.UnitData, error) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return nil, fmt.Errorf("unit %v does not contain non fungible token data", unitID)
				}
				d.data = attr.Data
				d.t = currentBlockNr
				d.backlink = txr.Hash(options.hashAlgorithm)
				return d, nil
			})); err != nil {
			return nil, err
		}

		return sm, nil
	}
}

func validateUpdateNonFungibleToken(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, s *state.State, hashAlgorithm crypto.Hash) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := types.UnitID(tx.UnitID())
	u, err := s.GetUnit(unitID, false)
	if err != nil {
		return err
	}
	data, ok := u.Data().(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, attr.Backlink) {
		return errors.New("invalid backlink")
	}
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		hashAlgorithm,
		s,
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.dataUpdatePredicate
		},
		func(d *nonFungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	predicates = append([]state.Predicate{data.dataUpdatePredicate}, predicates...)
	sigBytes, err := tx.Payload.BytesWithAttributeSigBytes(attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.DataUpdateSignatures, sigBytes)
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
