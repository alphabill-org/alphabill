package evm

import (
	"math/big"
	"reflect"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

var pubKey = [33]byte{0x03,
	0x4F, 0x93, 0xA5, 0x2E, 0xE2, 0x16, 0x4C, 0xFC, 0x48, 0xC2, 0x87, 0x62, 0x57, 0xCC, 0xF0, 0x43,
	0xAD, 0x71, 0xCF, 0x5A, 0x5D, 0x6B, 0x4B, 0x1F, 0x2F, 0x19, 0xA, 0xDC, 0xBF, 0x90, 0x1C, 0x56}

func bigIntFromString(t *testing.T, value string) *big.Int {
	t.Helper()
	i, b := new(big.Int).SetString(value, 10)
	require.True(t, b)
	return i
}

func Test_generateAddress(t *testing.T) {
	type args struct {
		pubBytes []byte
	}
	tests := []struct {
		name    string
		args    args
		want    common.Address
		wantErr bool
	}{
		{
			name:    "ok",
			args:    args{pubKey[:]},
			want:    common.HexToAddress("0x276B52B4808893d1e2Affd5310898818E8e7699d"),
			wantErr: false,
		},
		{
			name:    "nil",
			args:    args{nil},
			wantErr: true,
		},
		{
			name:    "random bytes",
			args:    args{pubBytes: common.FromHex("0x4a18f39d69cb1b2f7278345df2ba4d691470e908ec30ed69e6e45ea2fe864463db")},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateAddress(tt.args.pubBytes)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateAddress() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getAddressFromPredicateArg(t *testing.T) {
	type args struct {
		predArg []byte
	}
	tests := []struct {
		name       string
		args       args
		want       common.Address
		wantErrStr string
	}{
		{
			name:       "error - nil",
			args:       args{predArg: nil},
			wantErrStr: "failed to extract public key from fee credit owner proof, empty owner proof as input",
		},
		{
			name: "ok",
			args: args{predArg: templates.NewP2pkh256SignatureBytes(test.RandomBytes(65), pubKey[:])},
			want: common.HexToAddress("0x276B52B4808893d1e2Affd5310898818E8e7699d"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getAddressFromPredicateArg(tt.args.predArg)
			if tt.wantErrStr != "" {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("getAddressFromPredicateArg() got = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func Test_alphaToWeiAndBack(t *testing.T) {
	alpha := uint64(1)
	wei := alphaToWei(alpha)
	require.EqualValues(t, "10000000000", wei.String())
	mia := weiToAlpha(wei)
	require.EqualValues(t, alpha, mia)

	alpha = uint64(99)
	wei = alphaToWei(alpha)
	mia = weiToAlpha(wei)
	require.EqualValues(t, alpha, mia)
}

func Test_alphaToWei(t *testing.T) {
	type args struct {
		alpha uint64
	}
	tests := []struct {
		name string
		args args
		want *big.Int
	}{
		{
			name: "1 alpha is 10^10 wei",
			args: args{alpha: 1},
			want: bigIntFromString(t, "10000000000"),
		},
		{
			name: "10 alpha is 10 * 10^10 wei",
			args: args{alpha: 10},
			want: bigIntFromString(t, "100000000000"),
		},
		{
			name: "999 alpha is 999 * 10^10 wei",
			args: args{alpha: 999},
			want: bigIntFromString(t, "9990000000000"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := alphaToWei(tt.args.alpha); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("alphaToWei() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_weiToAlpha(t *testing.T) {
	type args struct {
		wei *big.Int
	}
	tests := []struct {
		name string
		args args
		want uint64
	}{
		{
			name: "1 wei is 0 alpha",
			args: args{wei: bigIntFromString(t, "1")},
			want: 0,
		},
		{
			name: "10^10-1 wei is 1 alpha",
			args: args{wei: bigIntFromString(t, "9999999999")},
			want: 1,
		},
		{
			name: "10^10 wei is 1 alpha",
			args: args{wei: bigIntFromString(t, "10000000000")},
			want: 1,
		},
		{
			name: "10^10+1 wei is 1 alpha",
			args: args{wei: bigIntFromString(t, "10000000001")},
			want: 1,
		},
		{
			name: "(1.5*10^10)-1 wei is 1 alpha",
			args: args{wei: bigIntFromString(t, "14999999999")},
			want: 1,
		},
		{
			name: "(1.5*10^10) wei is 2 alpha",
			args: args{wei: bigIntFromString(t, "15000000000")},
			want: 2,
		},
		{
			name: "(2*10^10)-1 wei is 2 alpha",
			args: args{wei: bigIntFromString(t, "19999999999")},
			want: 2,
		},
		{
			name: "2*10^10 wei is 2 alpha",
			args: args{wei: bigIntFromString(t, "20000000000")},
			want: 2,
		},
		{
			name: "12*10^10 wei is 1 alpha",
			args: args{wei: bigIntFromString(t, "120000000000")},
			want: 12,
		},
		{
			name: "default gas price",
			args: args{wei: new(big.Int).Mul(big.NewInt(DefaultGasPrice), big.NewInt(1000))},
			want: 21,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weiToAlpha(tt.args.wei); got != tt.want {
				t.Errorf("weiToAlpha() = %v, want %v", got, tt.want)
			}
		})
	}
}
