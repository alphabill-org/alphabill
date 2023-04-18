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

func TestFungibleToken_Subtyping_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedirW1 := createNewTokenWallet(t, backendUrl)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	symbol1 := "AB"
	// test subtyping
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	typeID13 := randomID(t)
	typeID14 := randomID(t)
	//push bool false, equal; to satisfy: 5100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause 0x53510087", backendUrl, symbol1, typeID11))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID11)
	//second type
	//--parent-type without --subtype-input gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X", backendUrl, symbol1, typeID12, "ptpkh", typeID11), "missing [subtype-input]")
	//--subtype-input without --parent-type also gives error
	execTokensCmdWithError(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --subtype-input %s", backendUrl, symbol1, typeID12, "ptpkh", "0x535100"), "missing [parent-type]")
	//inheriting the first one and setting subtype clause to ptpkh
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID12, "ptpkh", typeID11, "0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID12)
	//third type needs to satisfy both parents, immediate parent with ptpkh, grandparent with 0x535100
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID13, "true", typeID12, "ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID13)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID13)
	//4th type
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s --symbol %s --type %X --subtype-clause %s --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID14, "true", typeID13, "empty,ptpkh,0x535100"))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID14)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID14)
}

func TestFungibleToken_InvariantPredicate_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedirW1 := createNewTokenWallet(t, backendUrl)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	symbol1 := "AB"
	typeID11 := randomID(t)
	typeID12 := randomID(t)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %X --decimals 0 --inherit-bearer-clause %s", backendUrl, symbol1, typeID11, predicatePtpkh))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID11)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID11)
	//second type inheriting the first one and leaves inherit-bearer clause to default (true)
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible -r %s  --symbol %s --type %X --decimals 0 --parent-type %X --subtype-input %s", backendUrl, symbol1, typeID12, typeID11, predicateTrue))
	require.Eventually(t, testpartition.BlockchainContains(partition, func(tx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, typeID12)
	}), test.WaitDuration, test.WaitTick)
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID12)
	//mint
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible -r %s  --type %X --amount %v --mint-input %s,%s", backendUrl, typeID12, 1000, predicatePtpkh, predicatePtpkh))
	ensureTokenIndexed(t, ctx, client, w1key.PubKey, nil)
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='1000'")
	//send to w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 100 --address 0x%X -k 1 --inherit-bearer-input %s,%s", backendUrl, typeID12, w2key.PubKey, predicateTrue, predicatePtpkh))
	ensureTokenIndexed(t, ctx, client, w2key.PubKey, nil)
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='100'")
}

func TestFungibleTokens_Sending_Integration(t *testing.T) {
	partition, nodeDialUrl := startTokensPartition(t)

	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, _, _ := startTokensBackend(t, nodeDialUrl)

	w1, homedirW1 := createNewTokenWallet(t, backendUrl)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()

	w2, homedirW2 := createNewTokenWallet(t, backendUrl)
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()

	typeID1 := randomID(t)
	// fungible token types
	symbol1 := "AB"
	execTokensCmdWithError(t, homedirW1, "new-type fungible", "required flag(s) \"symbol\" not set")
	execTokensCmd(t, homedirW1, fmt.Sprintf("new-type fungible  --symbol %s -r %s --type %X --decimals 0", symbol1, backendUrl, typeID1))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list-types fungible -r %s", backendUrl)), "symbol=AB (fungible)")
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
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %X --amount 5", backendUrl, typeID1))
	execTokensCmd(t, homedirW1, fmt.Sprintf("new fungible  -r %s --type %X --amount 9", backendUrl, typeID1))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(5)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(9)), test.WaitDuration, test.WaitTick)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='5'", "amount='9'", "Symbol='AB'")
	// check w2 is empty
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible  -r %s", backendUrl)), "No tokens")
	// transfer tokens w1 -> w2
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w2key.PubKey)) //split (9=>6+3)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='5'", "amount='3'", "Symbol='AB'")
	execTokensCmd(t, homedirW1, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w2key.PubKey)) //transfer (5) + split (3=>2+1)
	//check immediately as tx must be confirmed
	verifyStdout(t, execTokensCmd(t, homedirW2, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='6'", "amount='5'", "amount='1'", "Symbol='AB'")
	//check what is left in w1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl))
	}, "amount='2'")
	//transfer back w2->w1 (AB-513)
	execTokensCmd(t, homedirW2, fmt.Sprintf("send fungible -r %s --type %X --amount 6 --address 0x%X -k 1", backendUrl, typeID1, w1key.PubKey))
	verifyStdout(t, execTokensCmd(t, homedirW1, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='2'", "amount='6'")
}

func TestWalletCreateFungibleTokenTypeAndTokenAndSendCmd_IntegrationTest(t *testing.T) {
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

	partition, nodeDialUrl := startTokensPartition(t)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, client, ctx := startTokensBackend(t, nodeDialUrl)

	w1, homedir := createNewTokenWallet(t, backendUrl)
	require.NotNil(t, w1)
	w1key, err := w1.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w1.Shutdown()
	w2, _ := createNewTokenWallet(t, backendUrl) // homedirW2
	w2key, err := w2.GetAccountManager().GetAccountKey(0)
	require.NoError(t, err)
	w2.Shutdown()
	typeID := util.Uint256ToBytes(uint256.NewInt(uint64(0x10)))
	symbol := "AB"
	name := "Long name for AB"
	// create type
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible  --symbol %s --name %s -r %s --type %X --decimals %v", symbol, name, backendUrl, typeID, decimals))
	ensureTokenTypeIndexed(t, ctx, client, w1key.PubKey, typeID)
	// non-existing id
	nonExistingTypeId := util.Uint256ToBytes(uint256.NewInt(uint64(0x11)))
	// verify error
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 3", backendUrl, nonExistingTypeId), fmt.Sprintf("failed to load type with id %X", nonExistingTypeId))
	// new token creation fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 0", backendUrl, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 00.000", backendUrl, typeID), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 00.0.00", backendUrl, typeID), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount .00", backendUrl, typeID), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount a.00", backendUrl, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 0.0a", backendUrl, typeID), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.1111", backendUrl, typeID), fmt.Sprintf("invalid precision"))
	// out of range because decimals = 3 the value is equal to 18446744073709551615000
	execTokensCmdWithError(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 18446744073709551615", backendUrl, typeID), fmt.Sprintf("out of range"))
	// creation succeeds
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 3", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.1", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.11", backendUrl, typeID))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible  -r %s --type %X --amount 1.111", backendUrl, typeID))
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(3000)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1100)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1110)), test.WaitDuration, test.WaitTick)
	require.Eventually(t, testpartition.BlockchainContains(partition, crit(1111)), test.WaitDuration, test.WaitTick)

	// test send fails
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 2 --address 0x%X -k 1", backendUrl, nonExistingTypeId, w2key.PubKey), fmt.Sprintf("failed to load type with id %X", nonExistingTypeId))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 0 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 000.000 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("0 is not valid amount"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 00.0.00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("more than one comma"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount .00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("missing integer part"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount a.00 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("invalid amount string"))
	execTokensCmdWithError(t, homedir, fmt.Sprintf("send fungible -r %s --type %X --amount 1.1111 --address 0x%X -k 1", backendUrl, typeID, w2key.PubKey), fmt.Sprintf("invalid precision"))
}

func TestFungibleTokens_CollectDust_Integration(t *testing.T) {
	_, nodeDialUrl := startTokensPartition(t)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	backendUrl, _, _ := startTokensBackend(t, nodeDialUrl)

	w1, homedir := createNewTokenWallet(t, backendUrl)
	require.NotNil(t, w1)
	w1.Shutdown()

	typeID1 := randomID(t)
	symbol1 := "AB"
	execTokensCmd(t, homedir, fmt.Sprintf("new-type fungible --symbol %s -r %s --type %X --decimals 0", symbol1, backendUrl, typeID1))
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list-types fungible -r %s", backendUrl)), "symbol=AB (fungible)")
	// mint tokens
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible -r %s --type %X --amount 300", backendUrl, typeID1))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible -r %s --type %X --amount 700", backendUrl, typeID1))
	execTokensCmd(t, homedir, fmt.Sprintf("new fungible -r %s --type %X --amount 1000", backendUrl, typeID1))
	//check w1
	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='300'", "amount='700'", "amount='1000'")
	// DC
	execTokensCmd(t, homedir, fmt.Sprintf("collect-dust -r %s", backendUrl))

	verifyStdout(t, execTokensCmd(t, homedir, fmt.Sprintf("list fungible -r %s", backendUrl)), "amount='2000'")
}
