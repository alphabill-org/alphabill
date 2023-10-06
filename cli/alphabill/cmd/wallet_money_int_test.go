package cmd

import (
	"context"
	"crypto"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
)

var (
	fcrID     = moneytx.NewFeeCreditRecordID(nil, []byte{1})
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
	initialBill := &moneytx.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	logF := logger.LoggerBuilder(t)
	network := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	apiAddr, moneyRestClient := startMoneyBackend(t, moneyPartition, initialBill)

	// create 2 wallets
	am1, homedir1 := createNewWallet(t)
	w1PubKey, _ := am1.GetPublicKey(0)
	am1.Close()

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	// create fee credit for initial bill transfer
	transferFC := testfc.CreateFeeCredit(t, initialBill.ID, fcrID, fcrAmount, network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1BalanceBilly := initialBill.Value - fcrAmount

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(w1PubKey, initialBill.ID, fcrID, w1BalanceBilly, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, logF, homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly = w1BalanceBilly - feeAmountAlpha*1e8
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
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(50 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w2BalanceBilly, 8)))

	// TS1.2: send bills back to wallet-1
	// create fee credit for wallet-2
	stdout = execWalletCmd(t, logF, homedir2, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, logF, homedir2, apiAddr, feeAmountAlpha*1e8-2, 0)

	// send wallet-2 bills back to wallet-1
	stdout = execWalletCmd(t, logF, homedir2, fmt.Sprintf("send --amount %s --address %s", amountToString(w2BalanceBilly, 8), hexutil.Encode(w1PubKey)))
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
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// verify wallet-1 account-2 received said bills
	acc2BalanceBilly := uint64(200 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, logF, homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 2, amountToString(acc2BalanceBilly, 8)))

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
	acc2FeeCreditString := amountToString(acc2FeeCredit, 8)
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))

	// TS3:
	// verify transaction is broadcast immediately without confirmation
	stdout = execWalletCmd(t, logF, homedir1, fmt.Sprintf("send -w false --amount 2 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully sent transaction(s)")

	w1TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w1PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w1TxHistory)
	require.Len(t, w1TxHistory, 6)

	w2TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w2PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w2TxHistory)
	require.Len(t, w2TxHistory, 2)
}

func waitForBalanceCLI(t *testing.T, logF LoggerFactory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, logF, homedir, "get-balance --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func waitForFeeCreditCLI(t *testing.T, logF LoggerFactory, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, logF, homedir, "fees list --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
