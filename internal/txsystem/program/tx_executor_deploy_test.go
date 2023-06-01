package program

import (
	"context"
	"crypto"
	_ "embed"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

//go:embed test_program/add_one.wasm
var addOneWasm []byte

func Test_handlePDeployTx(t *testing.T) {
	type args struct {
		ctx   context.Context
		state *rma.Tree
		tx    *types.TransactionOrder
		attr  *PDeployAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "invalid wasm",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         counterProgramUnitID.Bytes(),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgModule: []byte{0, 1, 2, 3},
					ProgParams: []byte{1, 2},
				},
			},
			wantErrStr: "failed to initiate VM with wasm source, invalid magic number",
		},
		{
			name: "ok for now, but must become more strict",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgModule: addOneWasm,
					ProgParams: []byte{1, 2},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := handlePDeployTx(tt.args.ctx, tt.args.state, DefaultProgramsSystemIdentifier, crypto.SHA256)(tt.args.tx, tt.args.attr, 10)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func Test_validateDeployTx(t *testing.T) {
	type args struct {
		tx   *types.TransactionOrder
		attr *PDeployAttributes
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "validate ok",
			args: args{
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgModule: []byte{0, 1, 2, 3},
					ProgParams: []byte{1, 2},
				},
			},
		},
		{
			name: "sys id does not match",
			args: args{
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       []byte{0, 0, 0, 0},
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgModule: []byte{0, 1, 2, 3},
					ProgParams: []byte{1, 2},
				},
			},
			wantErrStr: "tx system id does not match tx system id",
		},
		{
			name: "no program",
			args: args{
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgParams: []byte{1, 2},
				},
			},
			wantErrStr: "wasm module is missing",
		},
		{
			name: "no progParams",
			args: args{
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
				},
				attr: &PDeployAttributes{
					ProgModule: []byte{0, 1, 2, 3},
				},
			},
			wantErrStr: "program init data is missing",
		},
		{
			name: "owner proof set",
			args: args{
				tx: &types.TransactionOrder{
					Payload: &types.Payload{
						SystemID:       systemID,
						Type:           ProgramDeploy,
						UnitID:         testtransaction.RandomBytes(32),
						ClientMetadata: &types.ClientMetadata{Timeout: 10, MaxTransactionFee: 2},
					},
					OwnerProof: script.PredicateAlwaysFalse(),
				},
				attr: &PDeployAttributes{
					ProgModule: []byte{0, 1, 2, 3},
				},
			},
			wantErrStr: "owner proof present",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeployTx(tt.args.tx, tt.args.attr, DefaultProgramsSystemIdentifier)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
