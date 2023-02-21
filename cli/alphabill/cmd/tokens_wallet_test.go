package cmd

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"io/ioutil"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	tw "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestParsePredicateClause(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		clause    string
		predicate []byte
		index     uint64
		err       string
	}{
		{
			clause:    "",
			predicate: script.PredicateAlwaysTrue(),
		}, {
			clause: "foo",
			err:    "invalid predicate clause",
		},
		{
			clause:    "0x53510087",
			predicate: []byte{0x53, 0x51, 0x00, 0x87},
		},
		{
			clause:    "true",
			predicate: script.PredicateAlwaysTrue(),
		},
		{
			clause:    "false",
			predicate: script.PredicateAlwaysFalse(),
		},
		{
			clause: "ptpkh:",
			err:    "invalid predicate clause",
		},
		{
			clause:    "ptpkh",
			index:     uint64(0),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "invalid key number: 0",
		},
		{
			clause:    "ptpkh:2",
			index:     uint64(1),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause:    "ptpkh:0x0102",
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0X",
			err:    "invalid predicate clause",
		},
	}

	for _, tt := range tests {
		t.Run(tt.clause, func(t *testing.T) {
			mock.recordedIndex = 0
			predicate, err := parsePredicateClause(tt.clause, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, tt.predicate, predicate)
			require.Equal(t, tt.index, mock.recordedIndex)
		})
	}
}

func TestParsePredicateArgument(t *testing.T) {
	mock := &accountManagerMock{keyHash: []byte{0x1, 0x2}}
	tests := []struct {
		input string
		// expectations:
		result tokens.Predicate
		accKey uint64
		err    string
	}{
		{
			input:  "",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "empty",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "true",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "false",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x",
			result: script.PredicateArgumentEmpty(),
		},
		{
			input:  "0x5301",
			result: []byte{0x53, 0x01},
		},
		{
			input: "ptpkh:0",
			err:   "invalid key number: 0",
		},
		{
			input:  "ptpkh",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:1",
			accKey: uint64(1),
		},
		{
			input:  "ptpkh:10",
			accKey: uint64(10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			argument, err := parsePredicateArgument(tt.input, mock)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				if tt.accKey > 0 {
					require.Equal(t, tt.accKey, argument.AccountNumber)
				} else {
					require.Equal(t, tt.result, argument.Argument)
				}
			}
		})
	}
}

func TestDecodeHexOrEmpty(t *testing.T) {
	empty := []byte{}
	tests := []struct {
		input  string
		result []byte
		err    string
	}{
		{
			input:  "",
			result: empty,
		},
		{
			input:  "empty",
			result: empty,
		},
		{
			input:  "0x",
			result: empty,
		},
		{
			input: "0x534",
			err:   "odd length hex string",
		},
		{
			input: "0x53q",
			err:   "invalid byte",
		},
		{
			input:  "53",
			result: []byte{0x53},
		},
		{
			input:  "0x5354",
			result: []byte{0x53, 0x54},
		},
		{
			input:  "5354",
			result: []byte{0x53, 0x54},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			res, err := decodeHexOrEmpty(tt.input)
			if tt.err != "" {
				require.ErrorContains(t, err, tt.err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.result, res)
			}
		})
	}
}

func Test_amountToString(t *testing.T) {
	type args struct {
		amount    uint64
		decPlaces uint32
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Conversion ok - decimals 2",
			args: args{amount: 12345, decPlaces: 2},
			want: "123.45",
		},
		{
			name: "Conversion ok - decimals 1",
			args: args{amount: 12345, decPlaces: 1},
			want: "1234.5",
		},
		{
			name: "Conversion ok - decimals 0",
			args: args{amount: 12345, decPlaces: 0},
			want: "12345",
		},
		{
			name: "Conversion ok - decimals 7",
			args: args{amount: 12345, decPlaces: 5},
			want: "0.12345",
		},
		{
			name: "Conversion ok - decimals 9",
			args: args{amount: 12345, decPlaces: 9},
			want: "0.000012345",
		},
		{
			name: "Conversion ok - 99999 ",
			args: args{amount: 99999, decPlaces: 7},
			want: "0.0099999",
		},
		{
			name: "Conversion ok - 9000 ",
			args: args{amount: 9000, decPlaces: 5},
			want: "0.09000",
		},
		{
			name: "Conversion ok - 3 ",
			args: args{amount: 3, decPlaces: 2},
			want: "0.03",
		},
		{
			name: "Conversion ok - 3 ",
			args: args{amount: 3, decPlaces: 2},
			want: "0.03",
		},
		{
			name: "Conversion of max - 18446744073709551615 ",
			args: args{amount: 18446744073709551615, decPlaces: 8},
			want: "184467440737.09551615",
		},
		{
			name: "decimals out of bounds - 18446744073709551615 ",
			args: args{amount: 18446744073709551615, decPlaces: 32},
			want: "0.00000000000018446744073709551615",
		},
		{
			name: "Conversion ok - 2.30",
			args: args{amount: 230, decPlaces: 2},
			want: "2.30",
		},
		{
			name: "Conversion ok - 0.300",
			args: args{amount: 3, decPlaces: 3},
			want: "0.003",
		},
		{
			name: "Conversion ok - 100230, decimals 3 - ok",
			args: args{amount: 100230, decPlaces: 3},
			want: "100.230",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := amountToString(tt.args.amount, tt.args.decPlaces)
			if got != tt.want {
				t.Errorf("amountToString() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_stringToAmount(t *testing.T) {
	type args struct {
		amount   string
		decimals uint32
	}
	tests := []struct {
		name       string
		args       args
		want       uint64
		wantErrStr string
	}{
		{
			name:       "empty",
			args:       args{amount: "", decimals: 2},
			want:       0,
			wantErrStr: "invalid empty amount string",
		},
		{
			name: "100.23, decimals 2 - ok",
			args: args{amount: "100.23", decimals: 2},
			want: 10023,
		},
		{
			name:       "100.2.3 error - too many commas",
			args:       args{amount: "100.2.3", decimals: 2},
			want:       0,
			wantErrStr: "more than one comma",
		},
		{
			name:       ".30 error - no whole number",
			args:       args{amount: ".3", decimals: 2},
			want:       0,
			wantErrStr: "missing integer part",
		},
		{
			name:       "30. error - no fraction",
			args:       args{amount: "30.", decimals: 2},
			want:       0,
			wantErrStr: "missing fraction part",
		},
		{
			name:       "1.000, decimals 2 - error invalid precision",
			args:       args{amount: "1.000", decimals: 2},
			want:       0,
			wantErrStr: "invalid precision",
		},
		{
			name:       "in.300, decimals 3 - error not number",
			args:       args{amount: "in.300", decimals: 3},
			want:       0,
			wantErrStr: "invalid amount string \"in.300\": error conversion to uint64 failed",
		},
		{
			name:       "12.3c0, decimals 3 - error not number",
			args:       args{amount: "12.3c0", decimals: 3},
			want:       0,
			wantErrStr: "invalid amount string \"12.3c0\": error conversion to uint64 failed",
		},
		{
			name: "2.30, decimals 2 - ok",
			args: args{amount: "2.30", decimals: 2},
			want: 230,
		},
		{
			name: "0000000.3, decimals 2 - ok",
			args: args{amount: "0000000.3", decimals: 2},
			want: 30,
		},
		{
			name: "0.300, decimals 3 - ok",
			args: args{amount: "0.300", decimals: 3},
			want: 300,
		},
		{
			name: "100.23, decimals 3 - ok",
			args: args{amount: "100.23", decimals: 3},
			want: 100230,
		},
		{
			name:       "18446744073709551615.2 out of range - error",
			args:       args{amount: "18446744073709551615.2", decimals: 1},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name:       "18446744073709551616 out of range - error",
			args:       args{amount: "18446744073709551616", decimals: 0},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name:       "18446744073709551615 out of range - error",
			args:       args{amount: "18446744073709551615", decimals: 1},
			want:       0,
			wantErrStr: "value out of range",
		},
		{
			name: "18446744073709551615 max - ok",
			args: args{amount: "18446744073709551615", decimals: 0},
			want: 18446744073709551615,
		},
		{
			name: "184467440737.09551615 max with decimals - ok",
			args: args{amount: "184467440737.09551615", decimals: 8},
			want: 18446744073709551615,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stringToAmount(tt.args.amount, tt.args.decimals)
			if len(tt.wantErrStr) > 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
				require.Equal(t, uint64(0), got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber uint64
		expectedKind  twb.Kind
		expectedPass  string
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: 0, // all tokens
			expectedKind:  twb.Any,
		},
		{
			name:          "list all tokens, encrypted wallet",
			args:          []string{"--pn", "some pass phrase"},
			accountNumber: 0, // all tokens
			expectedKind:  twb.Any,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  twb.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: 0,
			expectedKind:  twb.Fungible,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  twb.Fungible,
		},
		{
			name:          "list account fungible tokens, encrypted wallet",
			args:          []string{"fungible", "--key", "4", "--pn", "some pass phrase"},
			accountNumber: 4,
			expectedKind:  twb.Fungible,
			expectedPass:  "some pass phrase",
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: 0,
			expectedKind:  twb.NonFungible,
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  twb.NonFungible,
		},
		{
			name:          "list account non-fungible tokens, encrypted walled",
			args:          []string{"non-fungible", "--key", "5", "--pn", "some pass phrase"},
			accountNumber: 5,
			expectedKind:  twb.NonFungible,
			expectedPass:  "some pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind twb.Kind, accountNumber *uint64) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) > 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestListTokensTypesCommandInputs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedKind twb.Kind
		expectedPass string
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: twb.Any,
		},
		{
			name:         "list all tokens, encrypted wallet",
			args:         []string{"--pn", "test pass phrase"},
			expectedKind: twb.Any,
			expectedPass: "test pass phrase",
		},
		{
			name:         "list all fungible tokens",
			args:         []string{"fungible"},
			expectedKind: twb.Fungible,
		},
		{
			name:         "list all fungible tokens, encrypted wallet",
			args:         []string{"fungible", "--pn", "test pass phrase"},
			expectedKind: twb.Fungible,
			expectedPass: "test pass phrase",
		},
		{
			name:         "list all non-fungible tokens",
			args:         []string{"non-fungible"},
			expectedKind: twb.NonFungible,
		},
		{
			name:         "list all non-fungible tokens, encrypted wallet",
			args:         []string{"non-fungible", "--pn", "test pass phrase"},
			expectedKind: twb.NonFungible,
			expectedPass: "test pass phrase",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind twb.Kind) error {
				require.Equal(t, tt.expectedKind, kind)
				if len(tt.expectedPass) != 0 {
					passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
					require.NoError(t, err)
					require.Equal(t, tt.expectedPass, passwordFromArg)
				}
				exec = true
				return nil
			})
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			require.NoError(t, err)
			require.True(t, exec)
		})
	}
}

func TestWalletCreateFungibleTokenTypeCmd_SymbolFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// missing symbol parameter
	_, err := execCommand(homedir, "token new-type fungible --decimals 3")
	require.ErrorContains(t, err, "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	_, err = execCommand(homedir, "flag needs an argument: --symbol")
	// there currently are no restrictions on symbol length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(homedir, "token new-type fungible --symbol \"@1\" --type")
	require.ErrorContains(t, err, "flag needs an argument: --type")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --type 011")
	require.ErrorContains(t, err, "invalid argument \"011\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --type foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--type\" flag")
	// there currently are no restrictions on type length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_DecimalsFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	// hidden parameter type (not a mandatory parameter)
	_, err := execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals")
	require.ErrorContains(t, err, "flag needs an argument: --decimals")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals foo")
	require.ErrorContains(t, err, "invalid argument \"foo\" for \"--decimals\" flag")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals -1")
	require.ErrorContains(t, err, "invalid argument \"-1\" for \"--decimals\"")
	_, err = execCommand(homedir, "token new-type fungible --symbol \"@1\" --decimals 9")
	require.ErrorContains(t, err, "argument \"9\" for \"--decimals\" flag is out of range, max value 8")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new fungible --type A8B")
	require.ErrorContains(t, err, "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new fungible --type nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte")
	_, err = execCommand(homedir, "token new fungible --amount 4")
	require.ErrorContains(t, err, "required flag(s) \"type\" not set")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new fungible --type A8BB")
	require.ErrorContains(t, err, "required flag(s) \"amount\" not set")
}

func TestWalletCreateNonFungibleTokenCmd_TypeFlag(t *testing.T) {
	type args struct {
		cmdParams string
	}
	tests := []struct {
		name       string
		args       args
		want       []byte
		wantErrStr string
	}{
		{
			name:       "missing token type parameter",
			args:       args{cmdParams: "token new non-fungible --data 12AB"},
			wantErrStr: "required flag(s) \"type\" not set",
		},
		{
			name:       "missing token type parameter has no value",
			args:       args{cmdParams: "token new non-fungible --type"},
			wantErrStr: "flag needs an argument: --type",
		},
		{
			name:       "type parameter is not hex encoded",
			args:       args{cmdParams: "token new non-fungible --type 11dummy"},
			wantErrStr: "invalid argument \"11dummy\" for \"--type\" flag",
		},
		{
			name:       "type parameter is odd length",
			args:       args{cmdParams: "token new non-fungible --type A8B08"},
			wantErrStr: "invalid argument \"A8B08\" for \"--type\" flag: encoding/hex: odd length hex string",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.args.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletCreateNonFungibleTokenCmd_TokenIdFlag(t *testing.T) {
	//token-identifier parameter is odd length
	homedir := createNewTestWallet(t)
	_, err := execCommand(homedir, "token new non-fungible --type A8B0 --token-identifier A8B09")
	require.ErrorContains(t, err, "invalid argument \"A8B09\" for \"--token-identifier\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new non-fungible --type A8B0 --token-identifier nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--token-identifier\" flag: encoding/hex: invalid byte")
}

func TestWalletCreateNonFungibleTokenCmd_DataFileFlag(t *testing.T) {
	data := make([]byte, maxBinaryFile64Kb+1)
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "token new non-fungible --type 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token new non-fungible --type 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token new non-fungible --type 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64Kb limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWalletUpdateNonFungibleTokenDataCmd_Flags(t *testing.T) {
	data := make([]byte, maxBinaryFile64Kb+1)
	tmpfile, err := ioutil.TempFile(t.TempDir(), "test")
	require.NoError(t, err)
	_, err = tmpfile.Write(data)
	require.NoError(t, err)

	tests := []struct {
		name       string
		cmdParams  string
		want       []byte
		wantErrStr string
	}{
		{
			name:       "both data and data-file specified",
			cmdParams:  "token update --token-identifier 12AB --data 1122aabb --data-file=/tmp/test/foo.bin",
			wantErrStr: "if any flags in the group [data data-file] are set none of the others can be; [data data-file] were all set",
		},
		{
			name:       "data-file not found",
			cmdParams:  "token update --token-identifier 12AB --data-file=/tmp/test/foo.bin",
			wantErrStr: "data-file read error: stat /tmp/test/foo.bin: no such file or directory",
		},
		{
			name:       "data-file too big",
			cmdParams:  "token update --token-identifier 12AB --data-file=" + tmpfile.Name(),
			wantErrStr: "data-file read error: file size over 64Kb limit",
		},
		{
			name:       "update nft: both data flags missing",
			cmdParams:  "token update --token-identifier 12AB",
			wantErrStr: "either of ['--data', '--data-file'] flags must be specified",
		},
		{
			name:       "update nft: token id missing",
			cmdParams:  "token update",
			wantErrStr: "required flag(s) \"token-identifier\" not set",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			homedir := createNewTestWallet(t)
			_, err := execCommand(homedir, tt.cmdParams)
			if len(tt.wantErrStr) != 0 {
				require.ErrorContains(t, err, tt.wantErrStr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// tokenID == nil means first token will be considered as success
func ensureTokenIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, ownerPubKey []byte, tokenID twb.TokenID) *twb.TokenUnit {
	var res *twb.TokenUnit
	require.Eventually(t, func() bool {
		offsetKey := ""
		var tokens []twb.TokenUnit
		var err error
		for {
			tokens, offsetKey, err = api.GetTokens(ctx, twb.Any, ownerPubKey, offsetKey, 0)
			require.NoError(t, err)
			for _, token := range tokens {
				if tokenID == nil {
					res = &token
					return true
				}
				if bytes.Equal(token.ID, tokenID) {
					res = &token
					return true
				}
			}
			if offsetKey == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func ensureTokenTypeIndexed(t *testing.T, ctx context.Context, api *client.TokenBackend, creatorPubKey []byte, typeID twb.TokenTypeID) *twb.TokenUnitType {
	var res *twb.TokenUnitType
	require.Eventually(t, func() bool {
		offsetKey := ""
		var types []twb.TokenUnitType
		var err error
		for {
			types, offsetKey, err = api.GetTokenTypes(ctx, twb.Any, creatorPubKey, offsetKey, 0)
			require.NoError(t, err)
			for _, t := range types {
				if bytes.Equal(t.ID, typeID) {
					res = &t
					return true
				}
			}
			if offsetKey == "" {
				break
			}
		}
		return false
	}, 2*test.WaitDuration, test.WaitTick)
	return res
}

func startTokensPartition(t *testing.T) *testpartition.AlphabillPartition {
	tokensState, err := rma.New(&rma.Config{
		HashAlgorithm: gocrypto.SHA256,
	})
	require.NoError(t, err)
	require.NotNil(t, tokensState)
	network, err := testpartition.NewNetwork(1,
		func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
			system, err := tokens.New(tokens.WithState(tokensState))
			require.NoError(t, err)
			return system
		}, tokens.DefaultTokenTxSystemIdentifier)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})
	startRPCServer(t, network, listenAddr)
	return network
}

func startTokensBackend(t *testing.T) (srvUri string, restApi *client.TokenBackend, ctx context.Context) {
	srvUri = defaultTokensBackendUri
	cfg := twb.NewConfig(defaultTokensBackendHost, dialAddr, filepath.Join(t.TempDir(), "backend.db"), func(a ...any) { fmt.Println(a...) })
	addr, err := url.Parse(srvUri)
	require.NoError(t, err)
	restApi = client.New(*addr)

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(context.Background())

	t.Cleanup(cancel)

	go func() {
		_ = twb.Run(ctx, cfg)
	}()

	require.Eventually(t, func() bool {
		rn, err := restApi.GetRoundNumber(ctx)
		require.NoError(t, err)
		return rn > 0
	}, test.WaitDuration, test.WaitTick)

	return
}

func createNewTokenWallet(t *testing.T, addr string) (*tw.Wallet, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	require.NoError(t, am.CreateKeys(""))

	w, err := tw.Load(addr, am)
	require.NoError(t, err)
	require.NotNil(t, w)

	return w, homeDir
}

func execTokensCmdWithError(t *testing.T, homedir string, command string, expectedError string) {
	_, err := doExecTokensCmd(homedir, command)
	require.ErrorContains(t, err, expectedError)
}

func execTokensCmd(t *testing.T, homedir string, command string) *testConsoleWriter {
	outputWriter, err := doExecTokensCmd(homedir, command)
	require.NoError(t, err)

	return outputWriter
}

func doExecTokensCmd(homedir string, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet token --log-level DEBUG --home " + homedir + " " + command // + " -l " + homedir + " "
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func randomID(t *testing.T) []byte {
	id, err := tw.RandomID()
	require.NoError(t, err)
	return id
}

func verifyStdoutEventually(t *testing.T, exec func() *testConsoleWriter, expectedLines ...string) {
	require.Eventually(t, func() bool {
		joined := strings.Join(exec().lines, "\n")
		res := true
		for _, expectedLine := range expectedLines {
			res = res && strings.Contains(joined, expectedLine)
		}
		return res
	}, test.WaitDuration, test.WaitTick)
}

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*account.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &account.AccountKey{PubKeyHash: &account.KeyHashes{Sha256: a.keyHash}}, nil
}

func (a *accountManagerMock) GetAll() []account.Account {
	return nil
}

func (a *accountManagerMock) CreateKeys(mnemonic string) error {
	return nil
}

func (a *accountManagerMock) AddAccount() (uint64, []byte, error) {
	return 0, nil, nil
}

func (a *accountManagerMock) GetMnemonic() (string, error) {
	return "", nil
}

func (a *accountManagerMock) GetAccountKeys() ([]*account.AccountKey, error) {
	return nil, nil
}

func (a *accountManagerMock) GetMaxAccountIndex() (uint64, error) {
	return 0, nil
}

func (a *accountManagerMock) GetPublicKey(accountIndex uint64) ([]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) GetPublicKeys() ([][]byte, error) {
	return nil, nil
}

func (a *accountManagerMock) IsEncrypted() (bool, error) {
	return false, nil
}

func (a *accountManagerMock) Close() {
}
