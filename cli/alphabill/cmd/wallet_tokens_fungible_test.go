package cmd

import (
	"fmt"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"testing"

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

	partition, unitState := startTokensPartition(t)
	startRPCServer(t, partition, listenAddr)
	require.NoError(t, wlog.InitStdoutLogger(wlog.INFO))

	w1, homedir := createNewTokenWallet(t, dialAddr)
	require.NotNil(t, w1)
	w1.Shutdown()
	w2, _ := createNewTokenWallet(t, dialAddr) // homedirW2
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
