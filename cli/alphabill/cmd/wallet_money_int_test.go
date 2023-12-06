package cmd

import (
	"context"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/testutils"
	testobserve "github.com/alphabill-org/alphabill/testutils/observability"
	"github.com/alphabill-org/alphabill/testutils/partition"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

var (
	fcrID     = money.NewFeeCreditRecordID(nil, []byte{1})
	fcrAmount = uint64(1e8)
)

/*
Prep: start network and money backend, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: wallet-2 sends transactions back to wallet-1
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 2.1: wallet-1 account 2 sends one transaction to wallet-1 account 3
Test scenario 3: wallet-1 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	// create 2 wallets
	am1, homedir1 := createNewWallet(t)
	w1AccKey, _ := am1.GetAccountKey(0)
	am1.Close()

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	initialBill := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: templates.NewP2pkh256BytesFromKey(w1AccKey.PubKey),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	logF := testobserve.NewFactory(t)
	_ = startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	apiAddr, moneyRestClient := startMoneyBackend(t, moneyPartition, initialBill)

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, logF, homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly := initialBill.Value - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, logF, homedir1, defaultAlphabillApiURL, feeAmountAlpha*1e8-2, 0)

	// TS1:
	// send two transactions to wallet-2
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("send --amount 50 --address 0x%x --alphabill-api-uri %s", w2PubKey, apiAddr))
	verifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		"Paid 0.000'000'01 fees for transaction(s)")

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= 50 * 1e8
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(50 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w2BalanceBilly, 8)))

	// TS1.2: send bills back to wallet-1
	// create fee credit for wallet-2
	stdout = execWalletCmd(t, logF, homedir2, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, logF, homedir2, apiAddr, feeAmountAlpha*1e8-2, 0)

	// send wallet-2 bills back to wallet-1
	stdout = execWalletCmd(t, logF, homedir2, fmt.Sprintf("send --amount %s --address %s", util.AmountToString(w2BalanceBilly, 8), hexutil.Encode(w1AccKey.PubKey)))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	waitForBalanceCLI(t, logF, homedir2, apiAddr, 0, 0)

	// verify wallet-1 balance is increased
	w1BalanceBilly += w2BalanceBilly
	waitForBalanceCLI(t, logF, homedir1, apiAddr, w1BalanceBilly, 0)

	// TS2:
	// add additional accounts to wallet 1
	pubKey2Hex := addAccount(t, logF, homedir1)
	pubKey3Hex := addAccount(t, logF, homedir1)

	// send two bills to wallet account 2
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("send -k 1 --amount 50,150 --address %s,%s --alphabill-api-uri %s", pubKey2Hex, pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 account-1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir1, fmt.Sprintf("get-balance -k 1 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, util.AmountToString(w1BalanceBilly, 8)))

	// verify wallet-1 account-2 received said bills
	acc2BalanceBilly := uint64(200 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 2, util.AmountToString(acc2BalanceBilly, 8)))

	// TS2.1:
	// create fee credit for account 2
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("fees add --amount %d -k 2", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	waitForFeeCreditCLI(t, logF, homedir1, apiAddr, feeAmountAlpha*1e8-2, 1)

	// send tx from account-2 to account-3
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("send --amount 100 --key 2 --address %s", pubKey3Hex))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalanceCLI(t, logF, homedir1, apiAddr, 100*1e8, 2)

	// verify account-2 fcb balance is reduced after send
	stdout = execWalletCmd(t, logF, homedir1, "fees list -k 2")
	acc2FeeCredit := feeAmountAlpha*1e8 - 3 // minus one for tx and minus one for creating fee credit
	acc2FeeCreditString := util.AmountToString(acc2FeeCredit, 8)
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))

	// TS3:
	// verify transaction is broadcast immediately without confirmation
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("send -w false --amount 2 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully sent transaction(s)")

	w1TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w1AccKey.PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w1TxHistory)
	require.Len(t, w1TxHistory, 5)

	w2TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w2PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w2TxHistory)
	require.Len(t, w2TxHistory, 2)
}

func waitForBalanceCLI(t *testing.T, logF Factory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, logF, homedir, "get-balance --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func waitForFeeCreditCLI(t *testing.T, logF Factory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, logF, homedir, "fees list --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := util.AmountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
