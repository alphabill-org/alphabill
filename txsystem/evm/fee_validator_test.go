package evm

import (
	"testing"

	"github.com/alphabill-org/alphabill/api/types"
	testfc "github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testsig "github.com/alphabill-org/alphabill/validator/internal/testutils/sig"
)

/*
func Test_checkFeeAccountBalance(t *testing.T) {
	tree := rma.NewWithSHA256()
	validatorFn := checkFeeAccountBalance(tree)

}*/

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
			args: args{testfc.NewCloseFC(t, testfc.NewCloseFCAttr())},
			want: true,
		},
		{
			name: "add fee credit - true",
			args: args{testfc.NewAddFC(t, signer, nil)},
			want: true,
		},
		{
			name: "nil - false",
			args: args{tx: &types.TransactionOrder{Payload: nil}},
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
