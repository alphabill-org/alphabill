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
	"github.com/holiman/uint256"
)

func handleJoinFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[JoinFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, currentBlockNr uint64) (*types.ServerMetadata, error) {
		logger.Debug("Processing Join Fungible Token tx: %v", tx)
		sum, err := validateJoinFungibleToken(tx, attr, options)
		if err != nil {
			return nil, fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := options.feeCalculator()

		// TODO calculate hash after setting server metadata
		h := tx.Hash(options.hashAlgorithm)

		// update state
		// disable fee handling if fee is calculated to 0 (used to temporarily disable fee handling, can be removed after all wallets are updated)
		var fcFunc rma.Action
		if options.feeCalculator() == 0 {
			fcFunc = func(tree *rma.Tree) error {
				return nil
			}
		} else {
			fcrID := util.BytesToUint256(tx.GetClientFeeCreditRecordID())
			fcFunc = fc.DecrCredit(fcrID, fee, h)
		}

		unitID := util.BytesToUint256(tx.UnitID())
		if err := options.state.AtomicUpdate(
			fcFunc,
			rma.UpdateData(unitID,
				func(data rma.UnitData) rma.UnitData {
					d, ok := data.(*fungibleTokenData)
					if !ok {
						// No change in case of incorrect data type.
						return data
					}
					return &fungibleTokenData{
						tokenType: d.tokenType,
						value:     sum,
						t:         currentBlockNr,
						backlink:  h,
					}
				}, h)); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee}, nil
	}
}

func validateJoinFungibleToken(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, options *Options) (uint64, error) {
	bearer, d, err := getFungibleTokenData(util.BytesToUint256(tx.UnitID()), options.state)
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
		tokenTypeID := util.Uint256ToBytes(d.tokenType)
		if !bytes.Equal(btxAttr.Type, tokenTypeID) {
			return 0, fmt.Errorf("the type of the burned source token does not match the type of target token: expected %X, got %X", tokenTypeID, btxAttr.Type)
		}

		if !bytes.Equal(btxAttr.Nonce, attr.Backlink) {
			return 0, fmt.Errorf("the source tokens weren't burned to join them to the target token: source %X, target %X", btxAttr.Nonce, attr.Backlink)
		}
		proof := proofs[i]

		if err = types.VerifyTxProof(proof, btx, options.trustBase, options.hashAlgorithm); err != nil {
			return 0, fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if !bytes.Equal(d.backlink, attr.Backlink) {
		return 0, fmt.Errorf("invalid backlink: expected %X, got %X", d.backlink, attr.Backlink)
	}
	predicates, err := getChainedPredicates[*fungibleTokenTypeData](
		options.state,
		d.tokenType,
		func(d *fungibleTokenTypeData) []byte {
			return d.invariantPredicate
		},
		func(d *fungibleTokenTypeData) *uint256.Int {
			return d.parentTypeId
		},
	)
	if err != nil {
		return 0, err
	}
	return sum, verifyOwnership(bearer, predicates, TokenOwnershipProver{tx: tx, invariantPredicateSignatures: attr.InvariantPredicateSignatures})
}
