package tokens

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/types"
)

func (m *FungibleTokensModule) handleJoinFungibleTokenTx() txsystem.GenericExecuteFunc[JoinFungibleTokenAttributes] {
	return func(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (*types.ServerMetadata, error) {
		sum, err := m.validateJoinFungibleToken(tx, attr, exeCtx)
		if err != nil {
			return nil, fmt.Errorf("invalid join fungible token tx: %w", err)
		}
		fee := m.feeCalculator()
		unitID := tx.UnitID()

		// update state
		if err := m.state.Apply(
			state.UpdateUnitData(unitID,
				func(data state.UnitData) (state.UnitData, error) {
					d, ok := data.(*FungibleTokenData)
					if !ok {
						return nil, fmt.Errorf("unit %v does not contain fungible token data", unitID)
					}
					return &FungibleTokenData{
						TokenType: d.TokenType,
						Value:     sum,
						T:         exeCtx.CurrentBlockNr,
						Counter:   d.Counter + 1,
						Locked:    0,
					}, nil
				},
			),
		); err != nil {
			return nil, err
		}
		return &types.ServerMetadata{ActualFee: fee, TargetUnits: []types.UnitID{unitID}, SuccessIndicator: types.TxStatusSuccessful}, nil
	}
}

func (m *FungibleTokensModule) validateJoinFungibleToken(tx *types.TransactionOrder, attr *JoinFungibleTokenAttributes, exeCtx *txsystem.TxExecutionContext) (uint64, error) {
	bearer, d, err := getFungibleTokenData(tx.UnitID(), m.state)
	if err != nil {
		return 0, err
	}
	transactions := attr.BurnTransactions
	proofs := attr.Proofs
	if len(transactions) != len(proofs) {
		return 0, fmt.Errorf("invalid count of proofs: expected %v, got %v", len(transactions), len(proofs))
	}
	sum := d.Value
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
		if !bytes.Equal(btxAttr.TypeID, d.TokenType) {
			return 0, fmt.Errorf("the type of the burned source token does not match the type of target token: expected %s, got %s", d.TokenType, btxAttr.TypeID)
		}
		if !bytes.Equal(btxAttr.TargetTokenID, tx.UnitID()) {
			return 0, fmt.Errorf("burn tx target token id does not match with join transaction unit id: burnTx %X, joinTx %X", btxAttr.TargetTokenID, tx.UnitID())
		}
		if btxAttr.TargetTokenCounter != attr.Counter {
			return 0, fmt.Errorf("burn tx target token counter does not match with join transaction counter: burnTx %d, joinTx %d", btxAttr.TargetTokenCounter, attr.Counter)
		}
		if err = types.VerifyTxProof(proofs[i], btx, m.trustBase, m.hashAlgorithm); err != nil {
			return 0, fmt.Errorf("proof is not valid: %w", err)
		}
	}
	if d.Counter != attr.Counter {
		return 0, fmt.Errorf("invalid counter: expected %X, got %X", d.Counter, attr.Counter)
	}

	if err = m.execPredicate(bearer, tx.OwnerProof, tx, exeCtx); err != nil {
		return 0, fmt.Errorf("evaluating bearer predicate: %w", err)
	}
	err = runChainedPredicates[*FungibleTokenTypeData](
		exeCtx,
		tx,
		d.TokenType,
		attr.InvariantPredicateSignatures,
		m.execPredicate,
		func(d *FungibleTokenTypeData) (types.UnitID, []byte) {
			return d.ParentTypeId, d.InvariantPredicate
		},
		m.state.GetUnit,
	)
	if err != nil {
		return 0, fmt.Errorf("token type InvariantPredicate: %w", err)
	}
	return sum, nil
}

func (j *JoinFungibleTokenAttributes) SigBytes() ([]byte, error) {
	// TODO: AB-1016 exclude InvariantPredicateSignatures from the payload hash because otherwise we have "chicken and egg" problem.
	signatureAttr := &JoinFungibleTokenAttributes{
		BurnTransactions:             j.BurnTransactions,
		Proofs:                       j.Proofs,
		Counter:                      j.Counter,
		InvariantPredicateSignatures: nil,
	}
	return types.Cbor.Marshal(signatureAttr)
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

func (j *JoinFungibleTokenAttributes) GetCounter() uint64 {
	return j.Counter
}

func (j *JoinFungibleTokenAttributes) SetCounter(counter uint64) {
	j.Counter = counter
}

func (j *JoinFungibleTokenAttributes) GetInvariantPredicateSignatures() [][]byte {
	return j.InvariantPredicateSignatures
}

func (j *JoinFungibleTokenAttributes) SetInvariantPredicateSignatures(signatures [][]byte) {
	j.InvariantPredicateSignatures = signatures
}
