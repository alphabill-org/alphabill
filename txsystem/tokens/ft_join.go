package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/api/types"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/validator/internal/state"
)

func handleJoinFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[JoinFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		sum, err := validateJoinFungibleToken(tx, attr, options)
		if err != nil {
			return nil, fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// update state
		unitID := tx.UnitID()
		h := tx.Hash(options.hashAlgorithm)
		if err := options.state.Apply(
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     sum,
						t:         currentBlockNr,
						backlink:  h,
						locked:    0,
					}, nil
				})); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func validateJoinFungibleToken(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, options *Options) (uint64, error) {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), options.state)
	if err != nil {
		return 0, err
	}
	transactions := attr.BurnTransactions
	proofs := attr.Proofs
	if len(transactions) != len(proofs) {
		return 0, fmt.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.value
	for i, btx := range transactions {
		prevSum := sum
		btxAttr := &BurnFungibleTokenAttributes{}
		if err := btx.TransactionOrder.UnmarshalAttributes(btxAttr); err != nil {
			return 0, fmt.Errorf("failed to unmarshal burn fungible token attributes")
		}

		sum += btxAttr.Value
		if prevSum > sum { // overflow
			return 0, errors.New("invalid sum of tokens: uint64 overflow")
		}
		if i > 0 && btx.TransactionOrder.UnitID().Compare(transactions[i-1].TransactionOrder.UnitID()) != 1 {
			// burning transactions orders are listed in strictly increasing order of token identifiers
			// this ensures that no source token can be included multiple times
			return 0, errors.New("burn tx orders are not listed in strictly increasing order of token identifiers")
		}
		if !bytes.Equal(btxAttr.TypeID, d.tokenType) {
			return 0, fmt.Errorf("the type of the burned source token does not match the type of target token: expected %s, got %s", d.tokenType, btxAttr.TypeID)
		}
		if !bytes.Equal(btxAttr.TargetTokenID, tx.UnitID()) {
			return 0, fmt.Errorf("burn tx target token id does not match with join transaction unit id: burnTx %X, joinTx %X", btxAttr.TargetTokenID, tx.UnitID())
		}
		if !bytes.Equal(btxAttr.TargetTokenBacklink, attr.Backlink) {
			return 0, fmt.Errorf("burn tx target token backlink does not match with join transaction backlink: burnTx %X, joinTx %X", btxAttr.TargetTokenBacklink, attr.Backlink)
		}
		if err = types.VerifyTxProof(proofs[i], btx, options.trustBase, options.hashAlgorithm); err != nil {
			return 0, fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if !bytes.Equal(d.backlink, attr.Backlink) {
		return 0, fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		options.hashAlgorithm,
		options.state,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) types.UnitID {
			return d.parentTypeId
		},
	)
	if err != nil {
		return 0, err
	}
	return sum, verifyOwnership(bearer, predicates, &joinFungibleTokenOwnershipProver{tx: tx, attr: attr})
}

type joinFungibleTokenOwnershipProver struct {
	tx   *types.TransactionOrder
	attr *JoinFungibleTokenAttributes
}

func (t *joinFungibleTokenOwnershipProver) OwnerProof() []byte {
	return t.tx.OwnerProof
}

func (t *joinFungibleTokenOwnershipProver) InvariantPredicateSignatures() [][]byte {
	return t.attr.InvariantPredicateSignatures
}

func (t *joinFungibleTokenOwnershipProver) SigBytes() ([]byte, error) {
	return t.tx.Payload.BytesWithAttributeSigBytes(t.attr)
}

func (j *JoinFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &JoinFungibleTokenAttributes{
		BurnTransactions:             j.BurnTransactions,
		Proofs:                       j.Proofs,
		Backlink:                     j.Backlink,
		InvariantPredicateSignatures: nil,
	}
	return cbor.Marshal(signatureAttr)
}

func (j *JoinFungibleTokenAttributes) GetBurnTransactions() []*types.TransactionRecord {
	return j.BurnTransactions
}

func (j *JoinFungibleTokenAttributes) SetBurnTransactions(burnTransactions []*types.TransactionRecord) {
	j.BurnTransactions = burnTransactions
}

func (j *JoinFungibleTokenAttributes) GetProofs() []*types.TxProof {
	return j.Proofs
}

func (j *JoinFungibleTokenAttributes) SetProofs(proofs []*types.TxProof) {
	j.Proofs = proofs
}

func (j *JoinFungibleTokenAttributes) GetBacklink() []byte {
	return j.Backlink
}

func (j *JoinFungibleTokenAttributes) SetBacklink(backlink []byte) {
	j.Backlink = backlink
}

func (j *JoinFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return j.InvariantPredicateSignatures
}

func (j *JoinFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	j.InvariantPredicateSignatures = signatures
}