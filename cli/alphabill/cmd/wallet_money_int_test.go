package cmd

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
)

/*
Prep: start network and money backend, send initial bill to wallet-1
Test scenario 1: wallet-1 sends two transactions to wallet-2
Test scenario 1.1: when sending a tx, wallet-1 specifies --output-path flag and checks proofs are saved there
Test scenario 2: wallet-1 account 1 sends two transactions to wallet-1 account 2
Test scenario 3: wallet-2 sends tx without confirming
*/
func TestSendingMoneyUsingWallets_integration(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
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

	am2, homedir2 := createNewWallet(t)
	w2PubKey, _ := am2.GetPublicKey(0)
	am2.Close()

	// create fee credit for initial bill transfer
	txFeeBilly := uint64(1)
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
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
		return execWalletCmd(t, "", homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(w1BalanceBilly, 8)))

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, "", homedir1, fmt.Sprintf("fees add --amount %d --alphabill-api-uri %s", feeAmountAlpha, apiAddr))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly = w1BalanceBilly - feeAmountAlpha*1e8 - txFeeBilly
	waitForFeeCreditCLI(t, homedir1, defaultAlphabillApiURL, feeAmountAlpha*1e8-1, 0)

	// TS1:
	// send two transactions (two bills) to wallet-2
	stdout = execWalletCmd(t, "", homedir1, fmt.Sprintf("send --amount 1 --address 0x%x --alphabill-api-uri %s", w2PubKey, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// TS1.1: also verify --output-path flag
	stdout = execWalletCmd(t, "", homedir1, fmt.Sprintf("send -k 1 --amount 1 --address 0x%x --alphabill-api-uri %s --output-path %s", w2PubKey, apiAddr, homedir1))
	proofFile := fmt.Sprintf("%s/bill-0x0000000000000000000000000000000000000000000000000000000000000001.json", homedir1)
	verifyStdout(t, stdout,
		"Successfully confirmed transaction(s)",
		fmt.Sprintf("Transaction proof(s) saved to: %s", proofFile),
	)
	require.FileExists(t, proofFile)

	// verify wallet-1 balance is decreased
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, "", homedir1, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(initialBill.Value-2e8, 8)))

	// verify wallet-2 received said bills
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, "", homedir2, fmt.Sprintf("get-balance --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(2e8, 8)))

	// TS2:
	// add additional accounts to wallet 1
	pubKey2Hex := addAccount(t, homedir1)
	_ = addAccount(t, homedir1)
	// send two transactions (two bills) to wallet account 2
	stdout = execWalletCmd(t, "", homedir1, fmt.Sprintf("send -k 1 --amount 1 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	stdout = execWalletCmd(t, "", homedir1, fmt.Sprintf("send -k 1 --amount 1 --address %s --alphabill-api-uri %s", pubKey2Hex, apiAddr))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, "", homedir1, fmt.Sprintf("get-balance -k 1 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 1, amountToString(initialBill.Value-4e8, 8)))

	// verify account 2 received said bills
	verifyStdoutEventually(t, func() *testConsoleWriter {
		return execWalletCmd(t, "", homedir1, fmt.Sprintf("get-balance -k 2 --alphabill-api-uri %s", apiAddr))
	}, fmt.Sprintf("#%d %s", 2, amountToString(2e8, 8)))

	// TS3:
	// verify transaction is broadcasted immediately
	stdout = execWalletCmd(t, "", homedir2, fmt.Sprintf("send -w false --amount 2 --address 0x%x --alphabill-api-uri %s", w1PubKey, apiAddr))
	verifyStdout(t, stdout, "Successfully sent transaction(s)")
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet-1
wallet-1 two transactions to wallet-2
wallet-2 sends transactions back to wallet-1
*/
func TestSendingMoneyBetweenWallets(t *testing.T) {
	wlog.InitStdoutLogger(wlog.DEBUG)
	// start alphabill partition
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
	abNet := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)
	alphabillNodeAddr := moneyPartition.Nodes[0].AddrGRPC

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            alphabillNodeAddr,
				ServerAddr:              defaultAlphabillApiURL, // TODO move to random port
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: initialBill.Owner,
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR},
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

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
	txFeeBilly := uint64(1)
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), abNet)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1BalanceBilly := initialBill.Value - fcrAmount - txFeeBilly

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(w1PubKey, initialBill.ID, w1BalanceBilly, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet 1
	waitForBalanceCLI(t, homedir1, defaultAlphabillApiURL, w1BalanceBilly, 0)

	// create fee credit for wallet-1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, alphabillNodeAddr, homedir1, fmt.Sprintf("fees add --amount %d", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	w1BalanceBilly = w1BalanceBilly - feeAmountAlpha*1e8 - txFeeBilly
	waitForFeeCreditCLI(t, homedir1, defaultAlphabillApiURL, feeAmountAlpha*1e8-1, 0)

	// send two transactions from wallet-1 to wallet-2
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir1, "send --amount 50 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// wait for backend to index the first transaction because
	// data on backend is slightly delayed from node (which we use to confirm tx)
	waitForBalanceCLI(t, homedir2, defaultAlphabillApiURL, 50*1e8, 0)

	stdout = execWalletCmd(t, alphabillNodeAddr, homedir1, "send --amount 150 --address "+hexutil.Encode(w2PubKey))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	w1BalanceBilly -= 200 * 1e8
	waitForBalanceCLI(t, homedir1, defaultAlphabillApiURL, w1BalanceBilly, 0)

	// verify wallet-2 received said bills
	w2BalanceBilly := uint64(200 * 1e8)
	waitForBalanceCLI(t, homedir2, defaultAlphabillApiURL, w2BalanceBilly, 0)

	// create fee credit for wallet-2
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir2, fmt.Sprintf("fees add --amount %d", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received for wallet-2
	w2BalanceBilly = w2BalanceBilly - feeAmountAlpha*1e8 - txFeeBilly
	waitForFeeCreditCLI(t, homedir2, defaultAlphabillApiURL, feeAmountAlpha*1e8-1, 0)

	// send wallet-2 bills back to wallet-1
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir2, fmt.Sprintf("send --amount %s --address %s", amountToString(w2BalanceBilly, 8), hexutil.Encode(w1PubKey)))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	waitForBalanceCLI(t, homedir2, defaultAlphabillApiURL, 0, 0)

	// verify wallet-1 balance is increased
	w1BalanceBilly += w2BalanceBilly
	waitForBalanceCLI(t, homedir1, defaultAlphabillApiURL, w1BalanceBilly, 0)
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet account 1
add two accounts to wallet
wallet account 1 sends two transactions to wallet account 2
wallet account 2 sends one transaction to wallet account 3
*/
func TestSendingMoneyBetweenWalletAccounts(t *testing.T) {
	wlog.InitStdoutLogger(wlog.DEBUG)
	// start alphabill partition
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
	abNet := startAlphabill(t, []*testpartition.NodePartition{moneyPartition})
	startPartitionRPCServers(t, moneyPartition)
	alphabillNodeAddr := moneyPartition.Nodes[0].AddrGRPC

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            alphabillNodeAddr,
				ServerAddr:              defaultAlphabillApiURL, // TODO move to random port
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: initialBill.Owner,
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR},
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// create wallet with 3 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()
	pubKey2Hex := addAccount(t, homedir)
	pubKey3Hex := addAccount(t, homedir)

	// create fee credit for initial bill transfer
	txFeeBilly := uint64(1)
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), abNet)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	acc1BalanceBilly := initialBill.Value - fcrAmount - txFeeBilly

	// transfer initial bill to wallet account 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKey1, initialBill.ID, acc1BalanceBilly, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = moneyPartition.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(moneyPartition, transferInitialBillTx), test.WaitDuration, test.WaitTick)

	// verify bill is received by account 1
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, acc1BalanceBilly, 0)

	// create fee credit for account 1
	feeAmountAlpha := uint64(1)
	stdout := execWalletCmd(t, alphabillNodeAddr, homedir, fmt.Sprintf("fees add --amount %d", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	acc1BalanceBilly = acc1BalanceBilly - feeAmountAlpha*1e8 - txFeeBilly
	waitForFeeCreditCLI(t, homedir, defaultAlphabillApiURL, feeAmountAlpha*1e8-txFeeBilly, 0)

	// send two transactions from account 1 to account 2
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir, "send --amount 50 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// wait for backend to index the first transaction because
	// data on backend is slightly delayed from node (which we use to confirm tx)
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, 50*1e8, 1)

	stdout = execWalletCmd(t, alphabillNodeAddr, homedir, "send --amount 150 --address "+pubKey2Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	acc1BalanceBilly -= 200 * 1e8
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, acc1BalanceBilly, 0)

	// verify account 2 balance is increased
	acc2BalanceBilly := uint64(200 * 1e8)
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, acc2BalanceBilly, 1)

	// create fee credit for account 2
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir, fmt.Sprintf("fees add --amount %d -k 2", feeAmountAlpha))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits on money partition.", feeAmountAlpha))

	// verify fee credit received
	waitForFeeCreditCLI(t, homedir, defaultAlphabillApiURL, feeAmountAlpha*1e8-txFeeBilly, 1)

	// send tx from account-2 to account-3
	stdout = execWalletCmd(t, alphabillNodeAddr, homedir, fmt.Sprintf("send --amount 100 --key 2 --address %s", pubKey3Hex))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, 100*1e8, 2)

	// verify account-2 fcb balance is reduced after send
	stdout = execWalletCmd(t, "", homedir, "fees list -k 2")
	acc2FeeCredit := feeAmountAlpha*1e8 - 2 // minus one for tx and minus one for creating fee credit
	acc2FeeCreditString := amountToString(acc2FeeCredit, 8)
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %s", acc2FeeCreditString))
}

func waitForBalanceCLI(t *testing.T, homedir string, url string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		stdout := execWalletCmd(t, "", homedir, "get-balance --alphabill-api-uri "+url)
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
		stdout := execWalletCmd(t, "", homedir, "fees list --alphabill-api-uri "+url)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("Account #%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}
