package contracts

import (
	"bytes"
	gocrypto "crypto"
	"reflect"
	"testing"

	"github.com/fxamacker/cbor/v2"

	"github.com/alphabill-org/alphabill/internal/types"

	"github.com/ethereum/go-ethereum/accounts/abi"

	test "github.com/alphabill-org/alphabill/internal/testutils"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/ethereum/go-ethereum/common"
)

func TestNewAlphabillLibContract(t *testing.T) {
	c := NewAlphabillLibContract(map[string]crypto.Verifier{}, gocrypto.SHA256)
	require.Len(t, c.gas, 1)
	require.Len(t, c.executor, 1)
	require.Equal(t, uint64(1000), c.RequiredGas(packInput(t, "verifyTxRecordProof", []byte{}, []byte{})))
	require.NotNil(t, c.executor["verifyTxRecordProof"])
}

func TestAlphabillLibPrecompiledContract_VerifyTransactionExecutionProof(t *testing.T) {
	type args struct {
		input  []byte
		caller common.Address
	}
	tests := []struct {
		name       string
		args       args
		want       []byte
		wantErrStr string
	}{
		{
			name: "method not found",
			args: args{
				input:  test.RandomBytes(20),
				caller: common.BytesToAddress(test.RandomBytes(20)),
			},
			wantErrStr: "invalid method",
		},
		{
			name: "unable to unpack input data",
			args: args{
				input:  append(packInput(t, "verifyTxRecordProof", []byte{}, []byte{}), []byte{1, 1, 1, 1}...),
				caller: common.BytesToAddress(test.RandomBytes(20)),
			},
			wantErrStr: "unable to unpack 'verifyTxRecordProof' input data",
		},
		{
			name: "invalid transaction record data",
			args: args{
				input:  packInput(t, "verifyTxRecordProof", []byte{}, []byte{}),
				caller: common.BytesToAddress(test.RandomBytes(20)),
			},
			wantErrStr: "invalid transaction record data",
			want:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "invalid transaction execution proof data",
			args: args{
				input:  packInput(t, "verifyTxRecordProof", emptyTransactionRecord(t), []byte{1, 1, 1}),
				caller: common.BytesToAddress(test.RandomBytes(20)),
			},
			wantErrStr: "invalid transaction execution proof data",
			want:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
		{
			name: "invalid transaction execution proof",
			args: args{
				input:  packInput(t, "verifyTxRecordProof", emptyTransactionRecord(t), emptyProof(t)),
				caller: common.BytesToAddress(test.RandomBytes(20)),
			},
			wantErrStr: "invalid transaction execution proof",
			want:       []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := NewAlphabillLibContract(map[string]crypto.Verifier{}, gocrypto.SHA256)
			got, err := d.Run(tt.args.input, tt.args.caller)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Run() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func emptyTransactionRecord(t *testing.T) []byte {
	txr := &types.TransactionRecord{}
	b, err := txr.Bytes()
	require.NoError(t, err)
	return b
}

func emptyProof(t *testing.T) []byte {
	p := &types.TxProof{}
	b, err := cbor.Marshal(p)
	require.NoError(t, err)
	return b
}

func packInput(t *testing.T, method string, args ...interface{}) []byte {
	alphabillLibABI, err := abi.JSON(bytes.NewBuffer([]byte(AlphabillLibABI)))
	require.NoError(t, err)
	data, err := alphabillLibABI.Pack(method, args...)
	require.NoError(t, err)
	return data
}
