package cmd

import (
	"context"
	gocrypto "crypto"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	tw "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type accountManagerMock struct {
	keyHash       []byte
	recordedIndex uint64
}

func (a *accountManagerMock) GetAccountKey(accountIndex uint64) (*wallet.AccountKey, error) {
	a.recordedIndex = accountIndex
	if accountIndex == 0 {
		return nil, errors.New("account does not exist")
	}
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
			index:     uint64(1),
			predicate: script.PredicatePayToPublicKeyHashDefault(mock.keyHash),
		},
		{
			clause: "ptpkh:0",
			err:    "account does not exist",
		},
		{
			clause:    "ptpkh:2",
			index:     uint64(2),
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

func TestTokens_withRunningPartition(t *testing.T) {
	addr := ":9543"
	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, addr)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1 := createNewTokenWallet(t, "w1", addr)
	require.NoError(t, w1.Sync(context.Background()))

	typeId1 := uint64(0x01)
	verifyStdout(t, execTokensCmd(t, "w1", ""), "Error: must specify a subcommand like new-type, send etc")
	verifyStdout(t, execTokensCmd(t, "w1", "new-type"), "Error: must specify a subcommand: fungible|non-fungible")

	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, "w1", "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X", symbol1, addr, util.Uint64ToBytes(typeId1)))
	ensureUnit(t, unitState, uint256.NewInt(typeId1))

	// non-fungible token types
	typeId2 := uint64(0x10)
	symbol2 := "ABNFT"
	execTokensCmdWithError(t, "w1", "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol2, addr, util.Uint64ToBytes(typeId2)))
	ensureUnit(t, unitState, uint256.NewInt(typeId2))
	fmt.Println("Finita")
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

func createNewTokenWallet(t *testing.T, name string, addr string) *tw.Wallet {
	mw := createNewNamedWallet(t, name, addr)

	w, err := tw.Load(mw, false)
	require.NoError(t, err)
	require.NotNil(t, w)

	return w
}

func execTokensCmdWithError(t *testing.T, walletName string, command string, expectedError string) {
	_, err := doExecTokensCmd(walletName, command)
	require.ErrorContains(t, err, expectedError)
}

func execTokensCmd(t *testing.T, walletName string, command string) *testConsoleWriter {
	outputWriter, err := doExecTokensCmd(walletName, command)
	require.NoError(t, err)

	return outputWriter
}

func doExecTokensCmd(walletName string, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{dumpToStdout: true}
	consoleWriter = outputWriter

	homeDir := path.Join(os.TempDir(), walletName)

	cmd := New()
	args := "wallet token --log-level DEBUG --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}
