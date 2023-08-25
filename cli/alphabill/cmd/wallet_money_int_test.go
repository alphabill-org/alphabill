package cmd

import (
	"context"
	"crypto"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

/*
Prep: start network and money backend, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: when sending a tx, wallet-1 specifies --output-path flag and checks proofs are saved there
Test scenario 1.2: wallet-2 sends transactions back to wallet-1
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 2.1: wallet-1 account 2 sends one transaction to wallet-1 account 3
Test scenario 3: wallet-1 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    util.Uint256ToBytes(uint256.NewInt(1)),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	network := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	apiAddr, moneyRestClient := startMoneyBackend(t, moneyPartition, initialBill)

	// create 2 wallets
	err := wlog.InitStdoutLogger(wlog.INFO)
	require.NoError(t, err)

	am1, homedir1 := createNewWallet(t)
	w1PubKey, _ := am1.GetPublicKey(0)
	am1.Close()

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	// create fee credit for initial bill transfer
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, initialBill.ID, network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1BalanceBilly := initialBill.Value - fcrAmount

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(w1PubKey, initialBill.ID, w1BalanceBilly, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly = w1BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, homedir1, defaultAlphabillApiURL, feeAmountAlpha*1e8-2, 0)

	// TS1:
	// send two transactions to wallet-2
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send --amount 50 --address 0x%x --alphabill-api-uri %s", w2PubKey, apiAddr))
	verifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		"Paid 0.000'000'01 fees for transaction(s)")

	// TS1.1: also verify --output-path flag
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 150 --address 0x%x --alphabill-api-uri %s --output-path %s", w2PubKey, apiAddr, homedir1))
	proofFile := fmt.Sprintf("%s/bill-0x0000000000000000000000000000000000000000000000000000000000000001.json", homedir1)
	verifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		fmt.Sprintf("Transaction proof(s) saved to: %s", proofFile),
		"Paid 0.000'000'01 fees for transaction(s)",
	)
	require.FileExists(t, proofFile)

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(200 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w2BalanceBilly, 8)))

	// TS1.2: send bills back to wallet-1
	// create fee credit for wallet-2
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8
	waitForFeeCreditCLI(t, homedir2, apiAddr, feeAmountAlpha*1e8-2, 0)

	// send wallet-2 bills back to wallet-1
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("send --amount %s --address %s", amountToString(w2BalanceBilly, 8), hexutil.Encode(w1PubKey)))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	waitForBalanceCLI(t, homedir2, apiAddr, 0, 0)

	// verify wallet-1 balance is increased
	w1BalanceBilly += w2BalanceBilly
	waitForBalanceCLI(t, homedir1, apiAddr, w1BalanceBilly, 0)

	// TS2:
	// add additional accounts to wallet 1
	pubKey2Hex := addAccount(t, homedir1)
	pubKey3Hex := addAccount(t, homedir1)

	// send two transactions to wallet account 2
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 50 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -k 1 --amount 150 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 account-1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance -k 1 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// verify wallet-1 account-2 received said bills
	acc2BalanceBilly := uint64(200 * 1e8)
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 2, amountToString(acc2BalanceBilly, 8)))

	// TS2.1:
	// create fee credit for account 2
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("fees add --amount %d -k 2", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	waitForFeeCreditCLI(t, homedir1, apiAddr, feeAmountAlpha*1e8-2, 1)

	// send tx from account-2 to account-3
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send --amount 100 --key 2 --address %s", pubKey3Hex))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalanceCLI(t, homedir1, apiAddr, 100*1e8, 2)

	// verify account-2 fcb balance is reduced after send
	stdout = execWalletCmd(t, homedir1, "fees list -k 2")
	acc2FeeCredit := feeAmountAlpha*1e8 - 3 // minus one for tx and minus one for creating fee credit
	acc2FeeCreditString := amountToString(acc2FeeCredit, 8)
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))

	// TS3:
	// verify transaction is broadcast immediately without confirmation
	stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send -w false --amount 2 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully sent transaction(s)")

	w1TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w1PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w1TxHistory)
	require.Len(t, w1TxHistory, 8)

	w2TxHistory, _, err := moneyRestClient.GetTxHistory(context.Background(), w2PubKey, "", 0)
	require.NoError(t, err)
	require.NotNil(t, w2TxHistory)
	require.Len(t, w2TxHistory, 4)
}

func TestMoneyDCUsingWallets_integration(t *testing.T) {
	t.SkipNow()
	initialBill := &moneytx.InitialBill{
		ID:    util.Uint256ToBytes(uint256.NewInt(1)),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 3)
	network := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)

	// start wallet backend
	apiAddr, _ := startMoneyBackend(t, moneyPartition, initialBill)

	// create 2 wallets
	err := wlog.InitStdoutLogger(wlog.INFO)
	require.NoError(t, err)

	am1, homedir1 := createNewWallet(t)
	w1PubKey, _ := am1.GetPublicKey(0)
	am1.Close()

	var wallet2Keys []sdk.PubKey
	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	wallet2Keys = append(wallet2Keys, w2PubKey)
	for i := 1; i < 20; i++ {
		idx, pubKey, _ := am2.AddAccount()
		require.EqualValues(t, i, idx)
		wallet2Keys = append(wallet2Keys, pubKey)
	}
	am2.Close()

	// create fee credit for initial bill transfer
	txFeeBilly := uint64(1)
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, initialBill.ID, network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1BalanceBilly := initialBill.Value - fcrAmount - txFeeBilly

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(w1PubKey, initialBill.ID, w1BalanceBilly, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly = w1BalanceBilly - feeAmountAlpha*1e8 - txFeeBilly
	waitForFeeCreditCLI(t, homedir1, defaultAlphabillApiURL, feeAmountAlpha*1e8-1, 0)

	amount := uint64(1000)
	// send money to all wallet-2 keys
	for _, pubKey := range wallet2Keys {
		stdout = execWalletCmd(t, homedir1, fmt.Sprintf("send --amount %v --address 0x%x --alphabill-api-uri %s", amount, pubKey, apiAddr))
		verifyStdout(t, stdout,
			"Successfully confirmed transaction(s)",
			"Paid 0.000'000'01 fees for transaction(s)")
	}

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= amount * uint64(len(wallet2Keys)) * 1e8
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// verify wallet-2 received said bills
	for idx := range wallet2Keys {
		w2BalanceBilly := amount * 1e8
		verifyStdoutEventually(t, func() *testConsoleWriter {
			return execWalletCmd(t, homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s -k %v", apiAddr, idx+1))
		}, fmt.Sprintf("#%d %s", idx+1, amountToString(w2BalanceBilly, 8)))
		// add fee credits
		stdout = execWalletCmd(t, homedir2, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s -k %v", feeAmountAlpha, apiAddr, idx+1))
		verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))
		waitForFeeCreditCLI(t, homedir2, apiAddr, feeAmountAlpha*1e8-1, uint64(idx))
	}

	for i := 0; i < 100; i++ {
		// transfer money from wallet-2 to wallet-1
		amount = uint64(1)
		for idx := range wallet2Keys {
			stdout = execWalletCmd(t, homedir2, fmt.Sprintf("send --amount %v --address 0x%x --alphabill-api-uri %s -k %v -w false", amount, w1PubKey, apiAddr, idx+1))
			verifyStdout(t, stdout, "Successfully sent transaction(s)")
		}
		w1BalanceBilly += amount * uint64(len(wallet2Keys)) * 1e8
		waitForBalanceCLI(t, homedir1, apiAddr, w1BalanceBilly, 0)
		fmt.Printf("wallet-1 current balance: %s\n", amountToString(w1BalanceBilly, 8))
	}

	// collect dust on wallet-1
	strOut := execWalletCmd(t, homedir1, fmt.Sprintf("collect-dust --alphabill-api-uri %s", apiAddr))
	verifyStdout(t, strOut, "Dust collection finished successfully.")
	fmt.Printf("collect-dust output: %s\n", strOut)

	// ensure wallet-1 balance did not change
	waitForBalanceCLI(t, homedir1, apiAddr, w1BalanceBilly, 0)
	fmt.Printf("wallet-1 balance after DC: %s\n", amountToString(w1BalanceBilly, 8))
}

func waitForBalanceCLI(t *testing.T, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, homedir, "get-balance --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func waitForFeeCreditCLI(t *testing.T, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, homedir, "fees list --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
