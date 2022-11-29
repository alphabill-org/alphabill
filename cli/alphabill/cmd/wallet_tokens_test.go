package cmd

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	tw "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/holiman/uint256"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	a.recordedIndex = accountIndex
	return &wallet.AccountKey{PubKeyHash: &wallet.KeyHashes{Sha256: a.keyHash}}, nil
}

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
			name: "100.23, decimals 3 - ok",
			args: args{amount: "100.230", decimals: 3},
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

func TestTokensWithRunningPartition(t *testing.T) {
	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, listenAddr)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	verifyStdout(t, execTokensCmd(t, homedirW1, ""), "Error: must specify a subcommand like new-type, send etc")
	verifyStdout(t, execTokensCmd(t, homedirW1, "new-type"), "Error: must specify a subcommand: fungible|non-fungible")

	testFungibleTokensWithRunningPartition(t, partition, unitState, w1key, w2key, homedirW1, homedirW2)

	testNFTsWithRunningPartition(t, partition, unitState, w2key, homedirW1, homedirW2)

	testTokenSubtypingWithRunningPartition(t, partition, unitState, w2key, homedirW1, homedirW2)
}

func testFungibleTokensWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w1key, w2key *wallet.AccountKey, homedirW1, homedirW2 string) {
	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X --decimals 2", symbol1, dialAddr, typeID1))
	ensureUnit(t, unitState, uint256.NewInt(0).SetBytes(typeID1))
	// mint tokens
	crit := func(amount uint64) func(tx *txsystem.Transaction) bool {
		return func(tx *txsystem.Transaction) bool {
			if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintFungibleTokenAttributes" {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 0.03", dialAddr, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 0.05", dialAddr, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 0.09", dialAddr, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(9)), test.WaitDuration, test.WaitTick)
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible --sync true -u %s", dialAddr)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -u %s --type %X --amount 0.06 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //split (9=>6+3)
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -u %s --type %X --amount 0.06 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)
	out := execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -u %s", dialAddr))
	verifyStdout(t, out, "amount='0.06'", "amount='0.05'", "amount='0.01'", "Symbol='AB'")
	verifyStdoutNotExists(t, out, "Symbol=''", "token-type=''")
	//check what is left in w1
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='0.03'", "amount='0.02'")
	//transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -u %s --type %X --amount 0.06 --address 0x%X -k 1", dialAddr, typeID1, w1key.PubKey))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='0.03'", "amount='0.02'", "amount='0.06'")
}

func testNFTsWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey, homedirW1, homedirW2 string) {
	// non-fungible token types
	typeID := randomID(t)
	nftID := randomID(t)
	symbol := "ABNFT"
	execTokensCmdWithError(t, homedirW1, "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol, dialAddr, typeID))
	ensureUnitBytes(t, unitState, typeID)
	// mint NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X", dialAddr, typeID, nftID))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	// transfer NFT
	execTokensCmd(t, homedirW1, fmt.Sprintf("send non-fungible --sync false -u %s --token-identifier %X --address 0x%X -k 1", dialAddr, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.TransferNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list non-fungible -u %s", dialAddr)), "No tokens")
	// list token types
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types")), "symbol=ABNFT (type,non-fungible)", "symbol=AB (type,fungible)")
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible")), "symbol=AB (type,fungible)")
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types non-fungible")), "symbol=ABNFT (type,non-fungible)")
}

func testTokenSubtypingWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey, homedirW1, homedirW2 string) {
	symbol1 := "AB"
	// test subtyping
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	typeID13 := randomID(t)
	typeID14 := randomID(t)
	//push bool false, equal; to satisfy: 5100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s", dialAddr, symbol1, typeID11, "0x53510087"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID11)
	//second type inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", dialAddr, symbol1, typeID12, "ptpkh", typeID11, "0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID12)
	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", dialAddr, symbol1, typeID13, "true", typeID12, "ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID13)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID13)
	//4th type
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", dialAddr, symbol1, typeID14, "true", typeID13, "empty,ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID14)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID14)
}

func TestListTokensCommandInputs(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		accountNumber int
		expectedKind  tw.TokenKind
	}{
		{
			name:          "list all tokens",
			args:          []string{},
			accountNumber: -1, // all tokens
			expectedKind:  tw.Any,
		},
		{
			name:          "list account tokens",
			args:          []string{"--key", "3"},
			accountNumber: 3,
			expectedKind:  tw.Any,
		},
		{
			name:          "list all fungible tokens",
			args:          []string{"fungible"},
			accountNumber: -1,
			expectedKind:  tw.FungibleToken,
		},
		{
			name:          "list account fungible tokens",
			args:          []string{"fungible", "--key", "4"},
			accountNumber: 4,
			expectedKind:  tw.FungibleToken,
		},
		{
			name:          "list all non-fungible tokens",
			args:          []string{"non-fungible"},
			accountNumber: -1,
			expectedKind:  tw.NonFungibleToken,
		},
		{
			name:          "list account non-fungible tokens",
			args:          []string{"non-fungible", "--key", "5"},
			accountNumber: 5,
			expectedKind:  tw.NonFungibleToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdList(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind tw.TokenKind, accountNumber *int) error {
				require.Equal(t, tt.accountNumber, *accountNumber)
				require.Equal(t, tt.expectedKind, kind)
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

func ensureUnitBytes(t *testing.T, state tokens.TokenState, id []byte) {
	ensureUnit(t, state, uint256.NewInt(0).SetBytes(id))
}

func ensureUnit(t *testing.T, state tokens.TokenState, id *uint256.Int) {
	unit, err := state.GetUnit(id)
	require.NoError(t, err)
	require.NotNil(t, unit)
}

func startTokensPartition(t *testing.T) (*testpartition.AlphabillPartition, tokens.TokenState) {
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
	return network, tokensState
}

func createNewTokenWallet(t *testing.T, addr string) (*tw.Wallet, string) {
	mw, homedir := createNewWallet(t, addr)

	w, err := tw.Load(mw, false)
	require.NoError(t, err)
	require.NotNil(t, w)

	return w, homedir
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
	args := "wallet token --log-level DEBUG --home " + homedir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func randomID(t *testing.T) tw.TokenID {
	id, err := tw.RandomID()
	require.NoError(t, err)
	return id
}

func TestListTokensTypesCommandInputs(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectedKind tw.TokenKind
	}{
		{
			name:         "list all tokens",
			args:         []string{},
			expectedKind: tw.Any,
		},
		{
			name:         "list all fungible tokens",
			args:         []string{"fungible"},
			expectedKind: tw.FungibleTokenType,
		},
		{
			name:         "list all non-fungible tokens",
			args:         []string{"non-fungible"},
			expectedKind: tw.NonFungibleTokenType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := false
			cmd := tokenCmdListTypes(&walletConfig{}, func(cmd *cobra.Command, config *walletConfig, kind tw.TokenKind) error {
				require.Equal(t, tt.expectedKind, kind)
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
