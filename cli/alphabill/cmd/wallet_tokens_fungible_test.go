package cmd

import (
	"bytes"
	"fmt"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

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

func TestFungibleTokenSubtyping_Integration(t *testing.T) {
	partition, unitState := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr)
	w1.Shutdown()

	verifyStdout(t, execTokensCmd(t, homedirW1, ""), "Error: must specify a subcommand like new-type, send etc")
	verifyStdout(t, execTokensCmd(t, homedirW1, "new-type"), "Error: must specify a subcommand: fungible|non-fungible")

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

func TestFungibleTokensCmd_Integration(t *testing.T) {
	partition, unitState := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X --decimals 2", symbol1, dialAddr, typeID1))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible")), "symbol=AB (type,fungible)")
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
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 3", dialAddr, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync false -u %s --type %X --amount 5", dialAddr, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 9", dialAddr, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(9)), test.WaitDuration, test.WaitTick)
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible --sync true -u %s", dialAddr)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //split (9=>6+3)
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)
	out := execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -u %s", dialAddr))
	verifyStdout(t, out, "amount='0.06'", "amount='0.05'", "amount='0.01'", "Symbol='AB'")
	verifyStdoutNotExists(t, out, "Symbol=''", "token-type=''")
	//check what is left in w1
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='0.03'", "amount='0.02'")
	//transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w1key.PubKey))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='0.03'", "amount='0.02'", "amount='0.06'")
}
