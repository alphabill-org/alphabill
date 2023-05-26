package program

import (
	"context"
	"crypto"
	_ "embed"
	"testing"

	"github.com/alphabill-org/alphabill/internal/rma"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/stretchr/testify/require"
)

//go:embed test_program/add_one.wasm
var addOneWasm []byte

func Test_handlePDeployTx(t *testing.T) {
	type args struct {
		ctx              context.Context
		state            *rma.Tree
		transactionOrder *PDeployTransactionOrder
		systemIdentifier []byte
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
				transactionOrder: newPDeployTxOrder(t, newDeployAttributes(WithProgram([]byte{0, 1, 2, 3}), WithInitData([]byte{1, 2})),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
				systemIdentifier: systemID,
			},
			wantErrStr: "program check failed, wasm program load failed, failed to initiate VM with wasm source, invalid magic number",
		},
		{
			name: "ok for now, but must become more strict",
			args: args{
				ctx:   context.Background(),
				state: rma.NewWithSHA256(),
				transactionOrder: newPDeployTxOrder(t, newDeployAttributes(WithProgram(addOneWasm), WithInitData([]byte{1, 2})),
					testtransaction.WithSystemID(systemID),
					testtransaction.WithOwnerProof(nil)),
				systemIdentifier: systemID,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handlePDeployTx(tt.args.ctx, tt.args.state, tt.args.systemIdentifier, crypto.SHA256)(tt.args.transactionOrder, 10)
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
		tx    *PDeployTransactionOrder
		sysID []byte
	}
	tests := []struct {
		name       string
		args       args
		wantErrStr string
	}{
		{
			name: "validate ok",
			args: args{
				tx: newPDeployTxOrder(t, newDeployAttributes(WithProgram([]byte{0, 1, 2, 3}), WithInitData([]byte{1, 2})),
					testtransaction.WithSystemID(systemID)),
				sysID: DefaultProgramsSystemIdentifier,
			},
		},
		{
			name: "sys id does not match",
			args: args{
				tx:    newPDeployTxOrder(t, newDeployAttributes(WithProgram([]byte{0, 1, 2, 3}), WithInitData([]byte{1, 2}))),
				sysID: DefaultProgramsSystemIdentifier,
			},
			wantErrStr: "tx system id does not match tx system id",
		},
		{
			name: "no program",
			args: args{
				tx: newPDeployTxOrder(t, newDeployAttributes(WithInitData([]byte{1, 2})),
					testtransaction.WithSystemID(systemID)),
				sysID: DefaultProgramsSystemIdentifier,
			},
			wantErrStr: "wasm module is missing",
		},
		{
			name: "no progParams",
			args: args{
				tx: newPDeployTxOrder(t, newDeployAttributes(WithProgram([]byte{0, 1, 2, 3})),
					testtransaction.WithSystemID(systemID)),
				sysID: DefaultProgramsSystemIdentifier,
			},
			wantErrStr: "program init data is missing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDeployTx(tt.args.tx, tt.args.sysID)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type PDOption func(*PDeployAttributes)

func WithProgram(wasm []byte) PDOption {
	return func(attr *PDeployAttributes) {
		attr.Program = wasm
	}
}

func WithInitData(data []byte) PDOption {
	return func(attr *PDeployAttributes) {
		attr.InitData = data
	}
}
func defaultDeployAttributes() *PDeployAttributes {
	return &PDeployAttributes{
		Program:  nil,
		InitData: nil,
	}
}

func newDeployAttributes(options ...PDOption) *PDeployAttributes {
	attr := defaultDeployAttributes()
	for _, o := range options {
		o(attr)
	}
	return attr
}

func newPDeployTxOrder(t *testing.T, pDeployAttr *PDeployAttributes, opts ...testtransaction.Option) *PDeployTransactionOrder {
	order := testtransaction.NewTransaction(t, opts...)
	return &PDeployTransactionOrder{
		txOrder:    order,
		attributes: pDeployAttr,
	}
}
