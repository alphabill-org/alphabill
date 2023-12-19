package testutils

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/require"
)

type InitialBill struct{
	ID    types.UnitID
	Value uint64
	Owner predicates.PredicateBytes
}

func MoneyGenesisState(t *testing.T, initialBill *InitialBill, dustCollectorMoneySupply uint64, sdrs []*genesis.SystemDescriptionRecord) *state.State {
	s := state.NewEmptyState()
	zeroHash := make([]byte, crypto.SHA256.Size())

	// initial bill
	require.NoError(t, s.Apply(state.AddUnit(initialBill.ID, initialBill.Owner, &money.BillData{V: initialBill.Value})))
	_, err := s.AddUnitLog(initialBill.ID, zeroHash)
	require.NoError(t, err)

	// dust collector money supply
	require.NoError(t, s.Apply(state.AddUnit(money.DustCollectorMoneySupplyID, money.DustCollectorPredicate, &money.BillData{V: dustCollectorMoneySupply})))
	_, err = s.AddUnitLog(money.DustCollectorMoneySupplyID, zeroHash)
	require.NoError(t, err)

	// fee credit bills
	for _, sdr := range sdrs {
		fcb := sdr.FeeCreditBill
		require.NoError(t, s.Apply(state.AddUnit(fcb.UnitId, fcb.OwnerPredicate, &money.BillData{})))
		// TODO: can't add unit log unless logs are added later also
		_, err = s.AddUnitLog(fcb.UnitId, zeroHash)
		require.NoError(t, err)
	}

	_, _, err = s.CalculateRoot()
	require.NoError(t, err)

	return s
}
