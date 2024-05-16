package fc

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckFeeCreditBalance(t *testing.T) {
	sharedState := state.NewEmptyState()
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	existingFCR := &fc.FeeCreditRecord{Balance: 10, Counter: 0, Locked: 1}
	require.NoError(t, sharedState.Apply(state.AddUnit(recordID, bearer, existingFCR)))
	require.NoError(t, sharedState.AddUnitLog(recordID, []byte{9}))
	fcModule, err := NewFeeCreditModule(
		WithSystemIdentifier(moneySystemID),
		WithMoneySystemIdentifier(moneySystemID),
		WithState(sharedState),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)

	tests := []struct {
		name          string
		tx            *types.TransactionOrder
		expectedError string
	}{
		{
			name:          "valid fee credit tx",
			tx:            testfc.NewAddFC(t, signer, nil),
			expectedError: "",
		},
		{
			name: "the tx fee cannot exceed the max specified fee",
			tx: testfc.NewAddFC(t, signer,
				testfc.NewAddFCAttr(t, signer),
				testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 0})),
			expectedError: "the tx fee cannot exceed the max specified fee",
		},
		{
			name:          "fee credit record missing",
			tx:            testtransaction.NewTransactionOrder(t, testtransaction.WithPayloadType("trans")),
			expectedError: "fee credit record missing",
		},
		{
			name: "fee credit record unit is nil",
			tx: testtransaction.NewTransactionOrder(t,
				testtransaction.WithPayloadType("trans"),
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1}}),
			),
			expectedError: "fee credit record unit is nil",
		},
		{
			name: "invalid fee proof",
			tx: testtransaction.NewTransactionOrder(t,
				testtransaction.WithPayloadType("trans"),
				testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
				testtransaction.WithOwnerProof(bearer),
			),
			expectedError: "decoding predicate",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := fcModule.CheckFeeCreditBalance(nil, test.tx)
			if test.expectedError != "" {
				assert.ErrorContains(t, err, test.expectedError, "unexpected error")
			} else {
				assert.NoError(t, err, "expected no error")
			}
		})
	}
}
