package orchestration

import (
	"crypto"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-sdk/types"

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
	txSystem, err := NewTxSystem(
		observability.Default(t),
		WithSystemIdentifier(orchestration.DefaultSystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithState(s),
		WithOwnerPredicate(templates.NewP2pkh256BytesFromKey(pubKey)),
	)
	require.NoError(t, err)
	require.NotNil(t, txSystem)

	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	roundNumber := uint64(10)
	txo := createAddVarTx(t, signer, orchestration.AddVarAttributes{},
		testtransaction.WithUnitID(unitID),
		testtransaction.WithClientMetadata(&types.ClientMetadata{Timeout: roundNumber + 1}),
	)

	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	serverMetadata, err := txSystem.Execute(txo)
	require.NoError(t, err)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))

	postCommitUnit, err := s.GetUnit(unitID, true)
	require.NoError(t, err)
	require.NotEqual(t, txo.OwnerProof, postCommitUnit.Bearer())
}

func createUC(s txsystem.StateSummary, roundNumber uint64) *types.UnicityCertificate {
	return &types.UnicityCertificate{
		InputRecord: &types.InputRecord{
			RoundNumber:  roundNumber,
			Hash:         s.Root(),
			SummaryValue: s.Summary(),
		},
	}
}
