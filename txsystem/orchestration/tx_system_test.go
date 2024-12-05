package orchestration

import (
	"crypto"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

func TestNewTxSystem_OK(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	s := state.NewEmptyState()
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	pdr := types.PartitionDescriptionRecord{
		Version:             1,
		NetworkID:   5,
		PartitionID: orchestration.DefaultPartitionID,
		TypeIdLen:           8,
		UnitIdLen:           256,
		T2Timeout:           2000 * time.Millisecond,
	}
	txSystem, err := NewTxSystem(
		pdr,
		types.ShardID{},
		observability.Default(t),
		WithHashAlgorithm(crypto.SHA256),
		WithState(s),
		WithOwnerPredicate(templates.NewP2pkh256BytesFromKey(pubKey)),
	)
	require.NoError(t, err)
	require.NotNil(t, txSystem)

	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	roundNumber := uint64(10)
	txo, _ := createAddVarTx(t, signer, &orchestration.AddVarAttributes{},
		testtransaction.WithUnitID(unitID),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: roundNumber + 1}),
	)

	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(txo)
	require.NoError(t, err)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{txo.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee == 0)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))
}

func createUC(s txsystem.StateSummary, roundNumber uint64) *types.UnicityCertificate {
	return &types.UnicityCertificate{
		Version: 1,
		InputRecord: &types.InputRecord{
			Version:      1,
			RoundNumber:  roundNumber,
			Hash:         s.Root(),
			SummaryValue: s.Summary(),
		},
	}
}
