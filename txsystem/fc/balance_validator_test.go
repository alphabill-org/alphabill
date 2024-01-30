package fc

import (
	"testing"

	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckFeeCreditBalance(t *testing.T) {
	sharedState := state.NewEmptyState()
	fixedFeeCalculator := FixedFee(1)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	existingFCR := &unit.FeeCreditRecord{Balance: 10, Backlink: nil, Locked: 1}
	require.NoError(t, sharedState.Apply(state.AddUnit(recordID, bearer, existingFCR)))
	require.NoError(t, sharedState.AddUnitLog(recordID, []byte{9}))

	tests := []struct {
		name          string
		ctx           *txsystem.TxValidationContext
		expectedError string
	}{
		{
			name: "valid fee credit tx",
			ctx: &txsystem.TxValidationContext{
				Tx: testutils.NewAddFC(t, signer, nil),
			},
			expectedError: "",
		},
		{
			name: "the tx fee cannot exceed the max specified fee",
			ctx: &txsystem.TxValidationContext{
				Tx: testutils.NewAddFC(t, signer,
					testutils.NewAddFCAttr(t, signer),
					testtransaction.WithClientMetadata(&types.ClientMetadata{MaxTransactionFee: 0})),
			},
			expectedError: "the tx fee cannot exceed the max specified fee",
		},
		{
			name: "fee credit record missing",
			ctx: &txsystem.TxValidationContext{
				Tx: testtransaction.NewTransactionOrder(t, testtransaction.WithPayloadType("trans")),
			},
			expectedError: "fee credit record missing",
		},
		{
			name: "fee credit record unit is nil",
			ctx: &txsystem.TxValidationContext{
				Tx: testtransaction.NewTransactionOrder(t,
					testtransaction.WithPayloadType("trans"),
					testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: []byte{1}}),
				),
			},
			expectedError: "fee credit record unit is nil",
		},
		{
			name: "invalid fee proof",
			ctx: &txsystem.TxValidationContext{
				Tx: testtransaction.NewTransactionOrder(t,
					testtransaction.WithPayloadType("trans"),
					testtransaction.WithClientMetadata(&types.ClientMetadata{FeeCreditRecordID: recordID}),
					testtransaction.WithOwnerProof(bearer),
				),
			},
			expectedError: "invalid fee proof",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			validator := checkFeeCreditBalance(sharedState, fixedFeeCalculator)

			err := validator(test.ctx)

			if test.expectedError != "" {
				assert.ErrorContains(t, err, test.expectedError, "unexpected error")
			} else {
				assert.NoError(t, err, "expected no error")
			}
		})
	}
}
