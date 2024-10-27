package evm

import (
	"testing"

	basetemplates "github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/stretchr/testify/require"
)

func Test_checkFeeAccountBalance(t *testing.T) {
	// generate keys
	signer, _ := testsig.CreateSignerAndVerifier(t)
	verifier, _ := signer.Verifier()
	publicKey, _ := verifier.MarshalPublicKey()

	// create tx
	txo := testutils.NewAddFC(t, signer, nil)
	ownerProof := testsig.NewAuthProofSignature(t, txo, signer)
	err := txo.SetAuthProof(fc.AddFeeCreditAuthProof{OwnerProof: ownerProof})
	require.NoError(t, err)

	// create state with 1 unit
	s := state.NewEmptyState()
	addr, _ := getAddressFromPredicateArg(ownerProof)
	unitID := addr.Bytes()
	fcr := fc.NewFeeCreditRecord(100, basetemplates.NewP2pkh256BytesFromKey(publicKey), 0)
	err = s.Apply(state.AddUnit(unitID, fcr))
	require.NoError(t, err)

	// create checkFeeAccountBalance function
	predEng, err := predicates.Dispatcher(templates.New())
	require.NoError(t, err)
	predicateRunner := predicates.NewPredicateRunner(predEng.Execute)
	validateFeeBalanceFn := checkFeeAccountBalanceFn(s, predicateRunner)

	// validate fee balance tx ok
	require.NoError(t, validateFeeBalanceFn(&TxValidationContext{
		Tx:        txo,
		state:     s,
		NetworkID: 5,
		SystemID:  1,
	}))
}

func Test_isFeeCreditTx(t *testing.T) {
	type args struct {
		tx *types.TransactionOrder
	}
	signer, _ := testsig.CreateSignerAndVerifier(t)

	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "close fee credit - true",
			args: args{testutils.NewCloseFC(t, signer, testutils.NewCloseFCAttr())},
			want: true,
		},
		{
			name: "add fee credit - true",
			args: args{testutils.NewAddFC(t, signer, nil)},
			want: true,
		},
		{
			name: "nil - false",
			args: args{tx: &types.TransactionOrder{Version: 1}},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isFeeCreditTx(tt.args.tx); got != tt.want {
				t.Errorf("isFeeCreditTx() = %v, want %v", got, tt.want)
			}
		})
	}
}
