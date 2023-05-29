package tokens

import (
	"bytes"
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

func handleUpdateNonFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[UpdateNonFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Update Non-Fungible Token tx: %v", tx)
		if err := validateUpdateNonFungibleToken(tx, attr, options.state); err != nil {
			return nil, fmt.Errorf("invalid update none-fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
			rma.UpdateData(unitID, func(data rma.UnitData) (newData rma.UnitData) {
				d, ok := data.(*nonFungibleTokenData)
				if !ok {
					return data
				}
				d.data = attr.Data
				d.t = currentBlockNr
				d.backlink = tx.Hash(options.hashAlgorithm)
				return data
			}, h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateUpdateNonFungibleToken(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes, state *rma.Tree) error {
	if len(attr.Data) > dataMaxSize {
		return fmt.Errorf("data exceeds the maximum allowed size of %v KB", dataMaxSize)
	}
	unitID := util.BytesToUint256(tx.UnitID())
	u, err := state.GetUnit(unitID)
	if err != nil {
		return err
	}
	data, ok := u.Data.(*nonFungibleTokenData)
	if !ok {
		return fmt.Errorf("unit %v is not a non-fungible token type", unitID)
	}
	if !bytes.Equal(data.backlink, attr.Backlink) {
		return errors.New("invalid backlink")
	}
	predicates, err := getChainedPredicates[*nonFungibleTokenTypeData](
		state,
		data.nftType,
		func(d *nonFungibleTokenTypeData) []byte {
			return d.dataUpdatePredicate
		},
		func(d *nonFungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return err
	}
	predicates = append([]Predicate{data.dataUpdatePredicate}, predicates...)
	sigBytes, err := getUpdateNFTSignedData(tx, attr)
	if err != nil {
		return err
	}
	return verifyPredicates(predicates, attr.DataUpdateSignatures, sigBytes)
}

func getUpdateNFTSignedData(tx *types.TransactionOrder, attr *UpdateNonFungibleTokenAttributes) ([]byte, error) {
	// exclude DataUpdateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &UpdateNonFungibleTokenAttributes{
		Data:                 attr.Data,
		Backlink:             attr.Backlink,
		DataUpdateSignatures: nil,
	}
	attrBytes, err := cbor.Marshal(signatureAttr)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal attributes: %w", err)
	}
	payload := &types.Payload{
		SystemID:       tx.Payload.SystemID,
		Type:           tx.Payload.Type,
		UnitID:         tx.Payload.UnitID,
		Attributes:     attrBytes,
		ClientMetadata: tx.Payload.ClientMetadata,
	}
	return payload.Bytes()
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
