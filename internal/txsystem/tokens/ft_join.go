package tokens

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

func handleJoinFungibleTokenTx(options *Options) txsystem.GenericExecuteFunc[*joinFungibleTokenWrapper] {
	return func(tx *joinFungibleTokenWrapper, currentBlockNr uint64) error {
		logger.Debug("Processing Join Fungible Token tx: %v", tx)
		sum, err := validateJoinFungibleToken(tx, options)
		if err != nil {
			return fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := options.feeCalculator()
		tx.transaction.ServerMetadata = &txsystem.ServerMetadata{Fee: fee}
		// update state
		fcrID := tx.transaction.GetClientFeeCreditRecordID()

		unitID := tx.UnitID()
		h := tx.Hash(options.hashAlgorithm)
		return options.state.AtomicUpdate(
			fc.DecrCredit(fcrID, fee, h),
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
				}, h))
	}
}

func validateJoinFungibleToken(tx *joinFungibleTokenWrapper, options *Options) (uint64, error) {
	d, err := getFungibleTokenData(tx.UnitID(), options.state)
	if err != nil {
		return 0, err
	}
	transactions := tx.burnTransactions
	proofs := tx.BlockProofs()
	if len(transactions) != len(proofs) {
		return 0, errors.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.value
	for i, btx := range transactions {
		prevSum := sum
		sum += btx.Value()
		if prevSum > sum { // overflow
			return 0, errors.New("invalid sum of tokens: uint64 overflow")
		}
		tokenTypeID := util.Uint256ToBytes(d.tokenType)
		if !bytes.Equal(btx.TypeID(), tokenTypeID) {
			return 0, errors.Errorf("the type of the burned source token does not match the type of target token: expected %X, got %X", tokenTypeID, btx.TypeID())
		}

		if !bytes.Equal(btx.Nonce(), tx.Backlink()) {
			return 0, errors.Errorf("the source tokens weren't burned to join them to the target token: source %X, target %X", btx.Nonce(), tx.Backlink())
		}
		proof := proofs[i]
		if proof.ProofType != block.ProofType_PRIM {
			return 0, errors.New("invalid proof type")
		}

		err = proof.Verify(util.Uint256ToBytes(btx.UnitID()), btx, options.trustBase, options.hashAlgorithm)
		if err != nil {
			return 0, errors.Wrap(err, "proof is not valid")
		}
	}
	if !bytes.Equal(d.backlink, tx.Backlink()) {
		return 0, errors.Errorf("invalid backlink: expected %X, got %X", d.backlink, tx.Backlink())
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
	return sum, verifyPredicates(predicates, tx.InvariantPredicateSignatures(), tx.SigBytes())
}
