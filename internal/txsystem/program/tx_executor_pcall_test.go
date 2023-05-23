package program

import (
	"context"
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
		ctx              context.Context
		state            *rma.Tree
		transactionOrder *PCallTransactionOrder
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "owner proof present",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(testtransaction.RandomBytes(64))),
			},
			wantErrStr: "owner proof present",
		},
		{
			name: "program not found",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(make([]byte, 32)),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in program not found",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(uint256.NewInt(0).PaddedBytes(32)),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in ok",
			args: args{
				ctx:   context.Background(),
				state: initStateWithBuiltInPrograms(t),
				transactionOrder: newPCallTxOrder(t, "count", testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlePCallTx(tt.args.ctx, tt.args.state, systemID, crypto.SHA256)(tt.args.transactionOrder, 10)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}

		})
	}
}

func newPCallTxOrder(t *testing.T, fName string, opts ...testtransaction.Option) *PCallTransactionOrder {
	order := testtransaction.NewTransaction(t, opts...)
	return &PCallTransactionOrder{
		txOrder: order,
		attributes: &PCallAttributes{
			Function: fName,
			Input:    []byte{00, 01},
		},
	}
}
