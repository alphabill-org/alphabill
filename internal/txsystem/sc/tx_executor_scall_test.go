package sc

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var systemID = []byte{0, 0, 0, 3}

func Test_handleSCallTx(t *testing.T) {
	type args struct {
		state    *state.State
		programs BuiltInPrograms
		tx       *types.TransactionOrder
		attr     *SCallAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "owner proof present",
			args: args{
				state: state.NewEmptyState(),
				tx:    newSCallTxOrder(t, testtransaction.WithOwnerProof(test.RandomBytes(64)), testtransaction.WithPayloadType("scall")),
				attr: &SCallAttributes{
					Input: test.RandomBytes(123),
				},
			},
			wantErrStr: "owner proof present",
		},
		{
			name: "program not found",
			args: args{
				state: state.NewEmptyState(),
				tx:    newSCallTxOrder(t, testtransaction.WithUnitId(make([]byte, 32)), testtransaction.WithOwnerProof(nil), testtransaction.WithPayloadType("scall")),
				attr: &SCallAttributes{
					Input: test.RandomBytes(123),
				},
			},
			wantErrStr: "failed to load program",
		},
		{
			name: "build-in program not found",
			args: args{
				state: initStateWithBuiltInPrograms(t),
				tx:    newSCallTxOrder(t, testtransaction.WithUnitId(uint256.NewInt(0).PaddedBytes(32)), testtransaction.WithOwnerProof(nil), testtransaction.WithPayloadType("scall")),
				attr: &SCallAttributes{
					Input: test.RandomBytes(123),
				},
			},
			wantErrStr: "failed to load program",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handleSCallTx(tt.args.state, tt.args.programs, systemID, crypto.SHA256)(tt.args.tx, tt.args.attr, 10)
			require.ErrorContains(t, err, tt.wantErrStr)
		})
	}
}

func newSCallTxOrder(t *testing.T, opts ...testtransaction.Option) *types.TransactionOrder {
	return testtransaction.NewTransactionOrder(t, opts...)
}
