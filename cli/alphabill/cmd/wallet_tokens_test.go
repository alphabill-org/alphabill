package cmd

import (
	"bytes"
	"context"
	gocrypto "crypto"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
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

func TestTokens_withRunningPartition(t *testing.T) {
	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, listenAddr)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1 := createNewTokenWallet(t, "w1", dialAddr)
	w1.Shutdown()
	w2 := createNewTokenWallet(t, "w2", dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	verifyStdout(t, execTokensCmd(t, "w1", ""), "Error: must specify a subcommand like new-type, send etc")
	verifyStdout(t, execTokensCmd(t, "w1", "new-type"), "Error: must specify a subcommand: fungible|non-fungible")

	testFungibleTokensWithRunningPartition(t, partition, unitState, w2key)

	testNFTsWithRunningPartition(t, partition, unitState, w2key)

	testTokenSubtypingWithRunningPartition(t, partition, unitState, w2key)
}

func testFungibleTokensWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	typeId1 := uint64(0x01)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, "w1", "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X", symbol1, dialAddr, util.Uint64ToBytes(typeId1)))
	ensureUnit(t, unitState, uint256.NewInt(typeId1))
	// mint tokens
	crit := func(amount uint64) func(tx *txsystem.Transaction) bool {
		return func(tx *txsystem.Transaction) bool {
			if tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintFungibleTokenAttributes" {
				attrs := &tokens.MintFungibleTokenAttributes{}
				require.NoError(t, tx.TransactionAttributes.UnmarshalTo(attrs))
				fmt.Printf("crit: %v vs amount %v", attrs.Value, amount)
				return attrs.Value == amount
			}
			return false
		}
	}
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 3", dialAddr, util.Uint64ToBytes(typeId1)))
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 5", dialAddr, util.Uint64ToBytes(typeId1)))
	execTokensCmd(t, "w1", fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 9", dialAddr, util.Uint64ToBytes(typeId1)))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(9)), test.WaitDuration, test.WaitTick)
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, "w2", fmt.Sprintf("list fungible --sync true -u %s", dialAddr)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, "w1", fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, util.Uint64ToBytes(typeId1), w2key.PubKey)) //split
	execTokensCmd(t, "w1", fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, util.Uint64ToBytes(typeId1), w2key.PubKey)) //transfer+split
	verifyStdout(t, execTokensCmd(t, "w2", fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='6'", "amount='5'", "amount='1'")
	//check what is left in w1
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='3'", "amount='2'")

}

func testNFTsWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	// non-fungible token types
	typeId2 := util.Uint256ToBytes(uint256.NewInt(uint64(0x10)))
	nftID := util.Uint256ToBytes(uint256.NewInt(uint64(0x11)))
	symbol2 := "ABNFT"
	execTokensCmdWithError(t, "w1", "new-type non-fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, "w1", fmt.Sprintf("new-type non-fungible --sync true --symbol %s -u %s --type %X", symbol2, dialAddr, typeId2))
	ensureUnitBytes(t, unitState, typeId2)
	// mint NFT
	execTokensCmd(t, "w1", fmt.Sprintf("new non-fungible --sync true -u %s --type %X --token-identifier %X", dialAddr, typeId2, nftID))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.MintNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	// transfer NFT
	execTokensCmd(t, "w1", fmt.Sprintf("send non-fungible --sync false -u %s --token-identifier %X --address 0x%X -k 1", dialAddr, nftID, w2key.PubKey))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return tx.TransactionAttributes.GetTypeUrl() == "type.googleapis.com/alphabill.tokens.v1.TransferNonFungibleTokenAttributes" && bytes.Equal(tx.UnitId, nftID)
	}), test.WaitDuration, test.WaitTick)
	verifyStdout(t, execTokensCmd(t, "w2", fmt.Sprintf("list non-fungible -u %s", dialAddr)), fmt.Sprintf("ID='%X'", nftID))
	//check what is left in w1, nothing, that is
	verifyStdout(t, execTokensCmd(t, "w1", fmt.Sprintf("list non-fungible -u %s", dialAddr)), "No tokens")
}

func testTokenSubtypingWithRunningPartition(t *testing.T, partition *testpartition.AlphabillPartition, unitState tokens.TokenState, w2key *wallet.AccountKey) {
	symbol1 := "AB"
	// test subtyping
	typeId11, err := tw.RandomId()
	typeId12, err := tw.RandomId()
	typeId13, err := tw.RandomId()
	typeId14, err := tw.RandomId()
	require.NoError(t, err)
	//push bool false, equal; to satisfy: 5100
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s", dialAddr, symbol1, typeId11, "0x53510087"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeId11)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeId11)
	//second type inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeId12, "ptpkh", typeId11, "0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeId12)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeId12)
	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeId13, "true", typeId12, "ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeId13)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeId13)
	//4th type
	execTokensCmd(t, "w1", fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause %s --parent-type %X --creation-input %s", dialAddr, symbol1, typeId14, "true", typeId13, "empty,ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeId14)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeId14)
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
