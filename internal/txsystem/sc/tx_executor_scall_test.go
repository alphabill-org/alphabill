package sc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var systemID = []byte{0, 0, 0, 3}

func Test_handleSCallTx(t *testing.T) {
	type args struct {
		state            *rma.Tree
		programs         BuiltInPrograms
		transactionOrder *SCallTransactionOrder
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "owner proof present",
			args: args{
				state:            rma.NewWithSHA256(),
				transactionOrder: newSCallTxOrder(t, testtransaction.WithOwnerProof(testtransaction.RandomBytes(64))),
			},
			wantErrStr: "owner proof present",
		},
		{
			name: "program not found",
			args: args{
				state:            rma.NewWithSHA256(),
				transactionOrder: newSCallTxOrder(t, testtransaction.WithUnitId(make([]byte, 32)), testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in program not found",
			args: args{
				state:            initStateWithBuiltInPrograms(t),
				transactionOrder: newSCallTxOrder(t, testtransaction.WithUnitId(uint256.NewInt(0).PaddedBytes(32)), testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handleSCallTx(tt.args.state, tt.args.programs, systemID, crypto.SHA256)(tt.args.transactionOrder, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func newSCallTxOrder(t *testing.T, opts ...testtransaction.Option) *SCallTransactionOrder {
	order := testtransaction.NewTransaction(t, opts...)

	return &SCallTransactionOrder{
		txOrder: order,
		attributes: &SCallAttributes{
			Input: testtransaction.RandomBytes(123),
		},
	}
}
