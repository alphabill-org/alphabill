package cmd

import (
	"bytes"
	"fmt"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestWalletCreateFungibleTokenTypeCmd_SymbolFlag(t *testing.T) {
	homedir := createNewTestWallet(t, createRandomTrustBase(t))
	// missing symbol parameter
	_, err := execCommand(homedir, "token new-type fungible --decimals 3")
	require.ErrorContains(t, err, "required flag(s) \"symbol\" not set")
	// symbol parameter not set
	_, err = execCommand(homedir, "flag needs an argument: --symbol")
	// there currently are no restrictions on symbol length on CLI side
}

func TestWalletCreateFungibleTokenTypeCmd_TypeIdlFlag(t *testing.T) {
	homedir := createNewTestWallet(t, createRandomTrustBase(t))
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
	homedir := createNewTestWallet(t, createRandomTrustBase(t))
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

func TestFungibleToken_Subtyping_Integration(t *testing.T) {
	partition, unitState, verifiers := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr, verifiers)
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
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --subtype-clause 0x53510087", dialAddr, symbol1, typeID11))
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

func TestFungibleToken_InvariantPredicate_Integration(t *testing.T) {
	partition, unitState, trustBase := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr, trustBase)
	_, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr, trustBase)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "AB"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --decimals 0 --inherit-bearer-clause %s", dialAddr, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -u %s --sync true --symbol %s --type %X --decimals 0 --parent-type %X --subtype-input %s", dialAddr, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureUnitBytes(t, unitState, typeID12)
	//mint
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible -u %s --sync true --type %X --amount %v --mint-input %s,%s", dialAddr, typeID12, 1000, predicatePtpkh, predicatePtpkh))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='1000'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -u %s --type %X --amount 100 --address 0x%X -k 1 --inherit-bearer-input %s,%s", dialAddr, typeID12, w2key.PubKey, predicateTrue, predicatePtpkh))
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='100'")
}

func TestFungibleTokens_Sending_Integration(t *testing.T) {
	partition, unitState, trustBase := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedirW1 := createNewTokenWallet(t, dialAddr, trustBase)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, dialAddr, trustBase)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X --decimals 0", symbol1, dialAddr, typeID1))
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
	verifyStdout(t, out, "amount='6'", "amount='5'", "amount='1'", "Symbol='AB'")
	verifyStdoutNotExists(t, out, "Symbol=''", "token-type=''")
	//check what is left in w1
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='3'", "amount='2'")
	//transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -u %s --type %X --amount 6 --address 0x%X -k 1", dialAddr, typeID1, w1key.PubKey))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -u %s", dialAddr)), "amount='3'", "amount='2'", "amount='6'")
}

func TestWalletCreateFungibleTokenCmd_TypeFlag(t *testing.T) {
	homedir := createNewTestWallet(t, createRandomTrustBase(t))
	_, err := execCommand(homedir, "token new fungible --type A8B")
	require.ErrorContains(t, err, "invalid argument \"A8B\" for \"--type\" flag: encoding/hex: odd length hex string")
	_, err = execCommand(homedir, "token new fungible --type nothex")
	require.ErrorContains(t, err, "invalid argument \"nothex\" for \"--type\" flag: encoding/hex: invalid byte")
	_, err = execCommand(homedir, "token new fungible --amount 4")
	require.ErrorContains(t, err, "required flag(s) \"type\" not set")
}

func TestWalletCreateFungibleTokenCmd_AmountFlag(t *testing.T) {
	homedir := createNewTestWallet(t, createRandomTrustBase(t))
	_, err := execCommand(homedir, "token new fungible --type A8BB")
	require.ErrorContains(t, err, "required flag(s) \"amount\" not set")
}

func TestWalletCreateFungibleTokenTypeAndTokenAndSendCmd_DataFileFlagIntegrationTest(t *testing.T) {
	const decimals = 3
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

	partition, unitState, trustBase := startTokensPartition(t)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedir := createNewTokenWallet(t, dialAddr, trustBase)
	require.NotNil(t, w1)
	w1.Shutdown()
	w2, _ := createNewTokenWallet(t, dialAddr, trustBase) // homedirW2
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()
	typeID := util.Uint256ToBytes(uint256.NewInt(uint64(0x10)))
	symbol := "AB"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible --sync true --symbol %s -u %s --type %X --decimals %v", symbol, dialAddr, typeID, decimals))
	ensureUnitBytes(t, unitState, typeID)
	// non-existing id
	nonExistingTypeId := util.Uint256ToBytes(uint256.NewInt(uint64(0x11)))
	// verify error
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 3", dialAddr, nonExistingTypeId), fmt.Sprintf("error token type %X not found", nonExistingTypeId))
	// new token creation fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 0", dialAddr, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 00.000", dialAddr, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 00.0.00", dialAddr, typeID), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount .00", dialAddr, typeID), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount a.00", dialAddr, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 0.0a", dialAddr, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 1.1111", dialAddr, typeID), fmt.Sprintf("invalid precision"))
	// out of range because decimals = 3 the value is equal to 18446744073709551615000
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 18446744073709551615", dialAddr, typeID), fmt.Sprintf("out of range"))
	// creation succeeds
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 3", dialAddr, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 1.1", dialAddr, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 1.11", dialAddr, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible --sync true -u %s --type %X --amount 1.111", dialAddr, typeID))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3000)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1100)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1110)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1111)), test.WaitDuration, test.WaitTick)

	// test send fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount 2 --address 0x%X -k 1", dialAddr, nonExistingTypeId, w2key.PubKey), fmt.Sprintf("error token type %X not found", nonExistingTypeId))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount 0 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount 000.000 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount 00.0.00 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount .00 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount a.00 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -u %s --type %X --amount 1.1111 --address 0x%X -k 1", dialAddr, typeID, w2key.PubKey), fmt.Sprintf("invalid precision"))
}
