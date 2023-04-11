package cmd

import (
	"context"
	"crypto"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	moneytestutils "github.com/alphabill-org/alphabill/internal/txsystem/money/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	testclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

type (
	backendMockReturnConf struct {
		balance        uint64
		blockHeight    uint64
		billId         *uint256.Int
		billValue      uint64
		billTxHash     string
		customBillList string
		customPath     string
		customFullPath string
		customResponse string
	}
)

func TestWalletCreateCmd(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	args := "wallet --home " + homeDir + " create"
	cmd.baseCmd.SetArgs(strings.Split(args, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
}

func TestWalletCreateCmd_encrypt(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	pw := "123456"
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" create --pn "+pw, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
	require.True(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
	verifyStdout(t, outputWriter,
		"The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")

	// verify wallet is encrypted
	// failing case: missing password
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key", " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// failing case: wrong password
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn 123", " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.ErrorContains(t, err, "invalid password")
	// passing case:
	cmd = New()
	cmd.baseCmd.SetArgs(strings.Split("wallet --home "+homeDir+" add-key --pn "+pw, " "))
	err = cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)
}

func TestWalletCreateCmd_invalidSeed(t *testing.T) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter
	homeDir := setupTestHomeDir(t, "wallet-test")

	cmd := New()
	cmd.baseCmd.SetArgs(strings.Split("wallet create -s --wallet-location "+homeDir, " "))
	err := cmd.addAndExecuteCommand(context.Background())
	require.EqualError(t, err, `invalid value "--wallet-location" for flag "seed" (mnemonic)`)
	require.False(t, util.FileExists(filepath.Join(homeDir, "wallet", "accounts.db")))
}

func TestWalletGetBalanceCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15", "Total 15")
}

func TestWalletGetBalanceKeyCmdKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	addAccount(t, homedir)
	stdout, _ := execCommand(homedir, "get-balance --key 2 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#2 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdTotalFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Total 15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestWalletGetBalanceCmdTotalWithKeyFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "get-balance --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdoutNotExists(t, stdout, "Total 15")
}

func TestWalletGetBalanceCmdQuietFlag(t *testing.T) {
	homedir := createNewTestWallet(t)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 15 * 1e8})
	defer mockServer.Close()

	// verify quiet flag does nothing if no key or total flag is not provided
	stdout, _ := execCommand(homedir, "get-balance --quiet --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15")
	verifyStdout(t, stdout, "Total 15")

	// verify quiet with total
	stdout, _ = execCommand(homedir, "get-balance --quiet --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")

	// verify quiet with key
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "Total 15")

	// verify quiet with key and total (total is not shown if key is provided)
	stdout, _ = execCommand(homedir, "get-balance --quiet --key 1 --total --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "15")
	verifyStdoutNotExists(t, stdout, "#1 15")
}

func TestPubKeysCmd(t *testing.T) {
	am, homedir := createNewWallet(t)
	pk, _ := am.GetPublicKey(0)
	am.Close()
	stdout, _ := execCommand(homedir, "get-pubkeys")
	verifyStdout(t, stdout, "#1 "+hexutil.Encode(pk))
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet-1
wallet-1 sends two transactions to wallet-2
wallet-2 swaps received bills
wallet-2 sends transaction back to wallet-1
*/
func TestSendingMoneyBetweenWallets(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

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
	txFee := uint64(1)
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet 1
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(w1PubKey, initialBill.ID, w1Balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	txHash, blockNumber := getLastTransactionProps(t, network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: w1Balance, blockHeight: blockNumber, billId: initialBill.ID, billValue: w1Balance, billTxHash: txHash})

	// verify bill is received by wallet 1
	waitForBalance(t, homedir1, w1Balance, 0)

	// create fee credit for wallet-1
	t.SkipNow() // TODO add fee credit management to wallet
	amount := uint64(100)
	stdout := execWalletCmd(t, homedir1, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir1, w1Balance, 0)

	// send two transactions (two bills) to wallet-2
	stdout = execWalletCmd(t, homedir1, "send --amount 50 --address "+hexutil.Encode(w2PubKey)+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	mockServer.Close()
	txHash, blockNumber = getLastTransactionProps(t, network)
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value - 100000000, billTxHash: txHash})
	defer mockServer.Close()

	stdout = execWalletCmd(t, homedir1, "send --amount 150 --address "+hexutil.Encode(w2PubKey)+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-1 balance is decreased
	w1Balance -= 200
	verifyAccountBalance(t, homedir1, w1Balance, 0)

	// verify wallet-2 received said bills
	w2Balance := uint64(200)
	waitForBalance(t, homedir2, w2Balance, 0)

	// create fee credit for wallet-2
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - amount - txFee
	waitForBalance(t, homedir2, w2Balance, 0)

	// swap wallet-2 bills
	stdout = execWalletCmd(t, homedir2, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// verify balance after swap remains the same
	verifyTotalBalance(t, homedir2, w2Balance)

	// send wallet-2 bill back to wallet-1
	stdout = execWalletCmd(t, homedir2, fmt.Sprintf("send --amount %d --address %s", w2Balance, hexutil.Encode(w1PubKey)))
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify wallet-2 balance is reduced
	verifyTotalBalance(t, homedir2, 0)

	// verify wallet-1 balance is increased
	w1Balance += w2Balance
	waitForBalance(t, homedir1, w1Balance, 0)
}

/*
Test scenario:
start network and rpc server and send initial bill to wallet account 1
add two accounts to wallet
wallet account 1 sends two transactions to wallet account 2
wallet runs dust collection
wallet account 2 sends transaction to account 3
*/
func TestSendingMoneyBetweenWalletAccounts(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 3 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()

	pubKey2Hex := addAccount(t, homedir)
	pubKey3Hex := addAccount(t, homedir)

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKey1, initialBill.ID, w1Balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, w1Balance, 0)

	// create fee credit for wallet-1
	t.SkipNow() // TODO add fee credit management to wallet
	amount := uint64(100)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir, w1Balance, 0)

	txHash, blockNumber := getLastTransactionProps(t, network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: txHash})

	// send two transactions (two bills) to wallet account 2
	stdout = execWalletCmd(t, homedir, "send --amount 50 --address "+pubKey2Hex+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	mockServer.Close()
	txHash, blockNumber = getLastTransactionProps(t, network)
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{balance: w1Balance, blockHeight: blockNumber, billId: initialBill.ID, billValue: w1Balance, billTxHash: txHash})
	defer mockServer.Close()

	stdout = execWalletCmd(t, homedir, "send --amount 150 --address "+pubKey2Hex+" --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")

	// verify account 1 balance is decreased
	w1Balance -= 200
	verifyAccountBalance(t, homedir, w1Balance, 0)

	// verify account 2 balance is increased
	w2Balance := uint64(200)
	waitForBalance(t, homedir, w2Balance, 1)

	// create fee credit for wallet-2
	w2FeeCredit := uint64(10)
	stdout = execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d -k 2", w2FeeCredit))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", w2FeeCredit))

	// verify balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - w2FeeCredit - txFee
	waitForBalance(t, homedir, w2Balance, 1)

	// swap account 2 bills
	stdout = execWalletCmd(t, homedir, "collect-dust")
	verifyStdout(t, stdout, "Dust collection finished successfully.")

	// verify balance after swap remains the same
	verifyAccountBalance(t, homedir, w2Balance, 1)

	// verify account-2 fcb balance is reduced after swap
	stdout = execWalletCmd(t, homedir, "fees list -k 2")
	w2FeeCredit = w2FeeCredit - 1 - 3 // 6 => added 10 credit minus 1 for transferFC minus 3 for 2x dcTx and 1x swap tx
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %d", w2FeeCredit))

	// send account-2 bills to account 3
	stdout = execWalletCmd(t, homedir, "send --amount 2 --key 2 --address "+pubKey3Hex)
	verifyStdout(t, stdout, "Successfully confirmed transaction(s)")
	waitForBalance(t, homedir, 2, 2)

	// verify account-2 fcb balance is reduced after send
	stdout = execWalletCmd(t, homedir, "fees list -k 2")
	w2FeeCredit = w2FeeCredit - 1
	verifyStdout(t, stdout, fmt.Sprintf("Account #2 %d", w2FeeCredit))
}

func TestSendWithoutWaitingForConfirmation(t *testing.T) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 3 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	am.Close()

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKey1, initialBill.ID, balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify bill is received by wallet account 1
	waitForBalance(t, homedir, balance, 0)

	// create fee credit for wallet account 1
	t.SkipNow() // TODO add fee credit management to wallet
	amount := uint64(10)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	balance = balance - amount - txFee
	verifyAccountBalance(t, homedir, balance, 0)

	_, bp, _ := network.GetBlockProof(transferInitialBillTx, txConverter)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: balance, blockHeight: bp.UnicityCertificate.GetRoundNumber(), billId: initialBill.ID, billValue: balance, billTxHash: base64.StdEncoding.EncodeToString(bp.TransactionsHash)})
	defer mockServer.Close()

	// verify transaction is broadcasted immediately
	stdout = execWalletCmd(t, homedir, "send -w false --amount 100 --address 0x00000046eed43bde3361e1a9ab6d0082dd923f20464a11869f8ef266045cf38d98 --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "Successfully sent transaction(s)")
}

func TestSendCmdOutputPathFlag(t *testing.T) {
	// setup network
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, ":9543")

	// create wallet with 2 accounts
	_ = wlog.InitStdoutLogger(wlog.DEBUG)
	am, homedir := createNewWallet(t)
	pubKey1, _ := am.GetPublicKey(0)
	pubKey1Hex := hexutil.Encode(pubKey1)
	am.Close()

	pubKey2Hex := addAccount(t, homedir)

	// create fee credit for initial bill transfer
	txFee := fc.FixedFee(1)()
	fcrAmount := testmoney.FCRAmount
	transferFC := testmoney.CreateFeeCredit(t, util.Uint256ToBytes(initialBill.ID), network)
	initialBillBacklink := transferFC.Hash(crypto.SHA256)
	w1Balance := initialBill.Value - fcrAmount - txFee

	// transfer initial bill to wallet
	transferInitialBillTx, err := moneytestutils.CreateInitialBillTransferTx(pubKey1, initialBill.ID, w1Balance, 10000, initialBillBacklink)
	require.NoError(t, err)
	err = network.SubmitTx(transferInitialBillTx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(transferInitialBillTx, network), test.WaitDuration, test.WaitTick)

	// verify tx is received by wallet
	waitForBalance(t, homedir, w1Balance, 0)

	// create fee credit for wallet-1
	t.SkipNow() // TODO add fee credit management to wallet
	amount := uint64(10)
	stdout := execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d", amount))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", amount))

	// verify original balance is reduced by fee credit amount + fee
	w1Balance = w1Balance - amount - txFee
	verifyAccountBalance(t, homedir, w1Balance, 0)

	txHash, blockNumber := getLastTransactionProps(t, network)
	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value, billTxHash: txHash})

	// send two transactions to wallet account 2 and verify the proof files
	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --alphabill-api-uri %s", 5, pubKey2Hex, homedir, addr.Host))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	mockServer.Close()
	txHash, blockNumber = getLastTransactionProps(t, network)
	mockServer, addr = mockBackendCalls(&backendMockReturnConf{balance: initialBill.Value, blockHeight: blockNumber, billId: initialBill.ID, billValue: initialBill.Value - 100000000, billTxHash: txHash})
	defer mockServer.Close()

	stdout, _ = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --alphabill-api-uri %s", 5, pubKey2Hex, homedir, addr.Host))
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], "Transaction proof(s) saved to: ")

	// verify wallet-2 balance is increased
	w2Balance := uint64(10)
	waitForBalance(t, homedir, w2Balance, 1)

	// create fee credit for wallet-2
	w2Fees := uint64(3)
	stdout = execWalletCmd(t, homedir, fmt.Sprintf("fees add --amount %d -k 2", w2Fees))
	verifyStdout(t, stdout, fmt.Sprintf("Successfully created %d fee credits.", w2Fees))

	// verify wallet-2 balance is reduced by fee credit amount + fee
	w2Balance = w2Balance - w2Fees - txFee
	verifyAccountBalance(t, homedir, w2Balance, 1)

	// verify wallet-2 send-all-balance outputs both bills
	stdout, err = execCommand(homedir, fmt.Sprintf("send --amount %d --address %s --output-path %s --key %d", w2Balance, pubKey1Hex, homedir, 2))
	require.NoError(t, err)
	require.Contains(t, stdout.lines[0], "Successfully confirmed transaction(s)")
	require.Contains(t, stdout.lines[1], fmt.Sprintf("Transaction proof(s) saved to: %s", filepath.Join(homedir, "bills.json")))
}

func TestSendingFailsWithInsufficientBalance(t *testing.T) {
	am, homedir := createNewWallet(t)
	pubKey, _ := am.GetPublicKey(0)
	am.Close()

	mockServer, addr := mockBackendCalls(&backendMockReturnConf{balance: 5e8})
	defer mockServer.Close()

	_, err := execCommand(homedir, "send --amount 10 --address "+hexutil.Encode(pubKey)+" --alphabill-api-uri "+addr.Host)
	require.ErrorIs(t, err, money.ErrInsufficientBalance)
}

func startAlphabillPartition(t *testing.T, initialBill *moneytx.InitialBill) *testpartition.AlphabillPartition {
	network, err := testpartition.NewNetwork(1, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := moneytx.NewMoneyTxSystem(
			defaultABMoneySystemIdentifier,
			moneytx.WithHashAlgorithm(crypto.SHA256),
			moneytx.WithInitialBill(initialBill),
			moneytx.WithSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
				{
					SystemIdentifier: defaultABMoneySystemIdentifier,
					T2Timeout:        defaultT2Timeout,
					FeeCreditBill: &genesis.FeeCreditBill{
						UnitId:         util.Uint256ToBytes(uint256.NewInt(2)),
						OwnerPredicate: script.PredicateAlwaysTrue(),
					},
				},
			}),
			moneytx.WithFeeCalculator(fc.FixedFee(1)),
			moneytx.WithDCMoneyAmount(10000),
			moneytx.WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, []byte{0, 0, 0, 0})
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = network.Close()
	})
	return network
}

func startRPCServer(t *testing.T, network *testpartition.AlphabillPartition, addr string) {
	// start rpc server for network.Nodes[0]
	listener, err := net.Listen("tcp", addr)
	require.NoError(t, err)

	grpcServer, err := initRPCServer(network.Nodes[0].Node, &grpcServerConfiguration{
		Address:               addr,
		MaxGetBlocksBatchSize: defaultMaxGetBlocksBatchSize,
		MaxRecvMsgSize:        defaultMaxRecvMsgSize,
		MaxSendMsgSize:        defaultMaxSendMsgSize,
	})
	require.NoError(t, err)

	t.Cleanup(func() {
		grpcServer.GracefulStop()
	})
	go func() {
		_ = grpcServer.Serve(listener)
	}()
}

func waitForBalance(t *testing.T, homedir string, expectedBalance uint64, accountIndex uint64) {
	require.Eventually(t, func() bool {
		mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
		defer mockServer.Close()

		stdout := execWalletCmd(t, homedir, "get-balance --alphabill-api-uri "+mockAddress.Host)
		for _, line := range stdout.lines {
			expectedBalanceStr := amountToString(expectedBalance, 8)
			if line == fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr) {
				return true
			}
		}
		return false
	}, test.WaitDuration, test.WaitTick)
}

func verifyTotalBalance(t *testing.T, walletName string, expectedBalance uint64) {
	mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
	defer mockServer.Close()
	stdout := execWalletCmd(t, walletName, "get-balance --alphabill-api-uri "+mockAddress.Host)
	expectedBalanceStr := amountToString(expectedBalance, 8)
	verifyStdout(t, stdout, fmt.Sprintf("Total %s", expectedBalanceStr))
}

func verifyAccountBalance(t *testing.T, walletName string, expectedBalance, accountIndex uint64) {
	mockServer, mockAddress := mockBackendCalls(&backendMockReturnConf{balance: expectedBalance})
	defer mockServer.Close()
	stdout := execWalletCmd(t, walletName, "get-balance --alphabill-api-uri "+mockAddress.Host)
	expectedBalanceStr := amountToString(expectedBalance, 8)
	verifyStdout(t, stdout, fmt.Sprintf("#%d %s", accountIndex+1, expectedBalanceStr))
}

// addAccount calls "add-key" cli function on given wallet and returns the added pubkey hex
func addAccount(t *testing.T, homedir string) string {
	stdout := execWalletCmd(t, homedir, "add-key")
	for _, line := range stdout.lines {
		if strings.HasPrefix(line, "Added key #") {
			return line[13:]
		}
	}
	return ""
}

func createNewWallet(t *testing.T) (account.Manager, string) {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	err = am.CreateKeys("")
	require.NoError(t, err)
	return am, homeDir
}

func createNewTestWallet(t *testing.T) string {
	homeDir := t.TempDir()
	walletDir := filepath.Join(homeDir, "wallet")
	am, err := account.NewManager(walletDir, "", true)
	require.NoError(t, err)
	defer am.Close()
	err = am.CreateKeys("")
	require.NoError(t, err)
	return homeDir
}

func verifyStdout(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	joined := strings.Join(consoleWriter.lines, "\n")
	for _, expectedLine := range expectedLines {
		require.Contains(t, joined, expectedLine)
	}
}

func verifyStdoutNotExists(t *testing.T, consoleWriter *testConsoleWriter, expectedLines ...string) {
	for _, expectedLine := range expectedLines {
		require.NotContains(t, consoleWriter.lines, expectedLine)
	}
}

func execCommand(homeDir, command string) (*testConsoleWriter, error) {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet --home " + homeDir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	return outputWriter, cmd.addAndExecuteCommand(context.Background())
}

func execWalletCmd(t *testing.T, homedir string, command string) *testConsoleWriter {
	outputWriter := &testConsoleWriter{}
	consoleWriter = outputWriter

	cmd := New()
	args := "wallet --home " + homedir + " " + command
	cmd.baseCmd.SetArgs(strings.Split(args, " "))

	err := cmd.addAndExecuteCommand(context.Background())
	require.NoError(t, err)

	return outputWriter
}

type testConsoleWriter struct {
	lines []string
}

func (w *testConsoleWriter) Println(a ...any) {
	s := fmt.Sprintln(a...)
	w.lines = append(w.lines, s[:len(s)-1]) // remove newline
}

func (w *testConsoleWriter) Print(a ...any) {
	w.Println(a...)
}

func mockBackendCalls(br *backendMockReturnConf) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == br.customPath || r.URL.RequestURI() == br.customFullPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(br.customResponse))
		} else {
			switch r.URL.Path {
			case "/" + testclient.BalancePath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"balance": "%d"}`, br.balance)))
			case "/" + testclient.BlockHeightPath:
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(fmt.Sprintf(`{"blockHeight": "%d"}`, br.blockHeight)))
			case "/" + testclient.ListBillsPath:
				w.WriteHeader(http.StatusOK)
				if br.customBillList != "" {
					w.Write([]byte(br.customBillList))
				} else {
					w.Write([]byte(fmt.Sprintf(`{"total": 1, "bills": [{"id":"%s","value":"%d","txHash":"%s","isDcBill":false}]}`, toBillId(br.billId), br.billValue, br.billTxHash)))
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}))

	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}

func toBillId(i *uint256.Int) string {
	return base64.StdEncoding.EncodeToString(util.Uint256ToBytes(i))
}

func txConverter(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
	return moneytx.NewMoneyTx([]byte{0, 0, 0, 0}, tx)
}

func getLastTransactionProps(t *testing.T, network *testpartition.AlphabillPartition) (string, uint64) {
	bl, err := network.Nodes[0].GetLatestBlock()
	require.NoError(t, err)
	_, bp, err := network.GetBlockProof(bl.Transactions[0], txConverter)
	require.NoError(t, err)
	txHash := base64.StdEncoding.EncodeToString(bp.GetTransactionsHash())
	return txHash, bp.UnicityCertificate.GetRoundNumber()
}
