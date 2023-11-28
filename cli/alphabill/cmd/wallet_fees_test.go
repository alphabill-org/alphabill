package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/predicates/templates"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
)

var defaultTokenSDR = &genesis.SystemDescriptionRecord{
	SystemIdentifier: tokens.DefaultSystemIdentifier,
	T2Timeout:        defaultT2Timeout,
	FeeCreditBill: &genesis.FeeCreditBill{
		UnitId:         money.NewBillID(nil, []byte{3}),
		OwnerPredicate: templates.AlwaysTrueBytes(),
	},
}

func TestWalletFeesCmds_MoneyPartition(t *testing.T) {
	logF := logger.LoggerBuilder(t)
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{})

	// list fees
	stdout, err := execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// add more fee credits
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees = amount*2*1e8 - 5 // minus 2 for first run, minus 3 for second run
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim fees
	stdout, err = execFeesCommand(logF, homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// add entire bill value worth of fees
	entireBillAmount := getFirstBillValue(t, homedir)
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%s", entireBillAmount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %s fee credits on money partition.", entireBillAmount), stdout.lines[0])
}

func TestWalletFeesCmds_TokenPartition(t *testing.T) {
	// start money partition and create wallet with token partition as well
	tokensPartition := createTokensPartition(t)
	logF := logger.LoggerBuilder(t)
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{tokensPartition})

	// start token partition
	startPartitionRPCServers(t, tokensPartition)

	tokenBackendURL, _, _ := startTokensBackend(t, tokensPartition.Nodes[0].AddrGRPC)
	args := fmt.Sprintf("--partition=tokens -r %s -m %s", defaultAlphabillApiURL, tokenBackendURL)

	// list fees on token partition
	stdout, err := execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)

	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 2
	stdout, err = execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// add more fee credits to token partition
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify fee credits to token partition
	expectedFees = amount*2*1e8 - 5
	stdout, err = execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim fees
	// invalid transaction: fee credit record unit is nil
	stdout, err = execFeesCommand(logF, homedir, "reclaim "+args)
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on tokens partition.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(logF, homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 2
	stdout, err = execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])
}

func TestWalletFeesCmds_MinimumFeeAmount(t *testing.T) {
	logF := logger.LoggerBuilder(t)
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{})

	// try to add invalid fee amount
	_, err := execFeesCommand(logF, homedir, "add --amount=0.00000003")
	require.Errorf(t, err, "minimum fee credit amount to add is %d", amountToString(fees.MinimumFeeAmount, 8))

	// add minimum fee amount
	stdout, err := execFeesCommand(logF, homedir, "add --amount=0.00000004")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000004 fee credits on money partition.", stdout.lines[0])

	// verify fee credit is below minimum
	expectedFees := uint64(2)
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim with invalid amount
	stdout, err = execFeesCommand(logF, homedir, "reclaim")
	require.Errorf(t, err, "insufficient fee credit balance. Minimum amount is %d", amountToString(fees.MinimumFeeAmount, 8))
	require.Empty(t, stdout.lines)

	// add more fee credit
	stdout, err = execFeesCommand(logF, homedir, "add --amount=0.00000005")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000005 fee credits on money partition.", stdout.lines[0])

	// verify fee credit is valid for reclaim
	expectedFees = uint64(4)
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// now we have enough credit to reclaim
	stdout, err = execFeesCommand(logF, homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.lines[0])
}

func TestWalletFeesLockCmds_Ok(t *testing.T) {
	logF := logger.LoggerBuilder(t)
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{})

	// create fee credit bill by adding fee credit
	stdout, err := execFeesCommand(logF, homedir, "add --amount 1")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 1 fee credits on money partition.", stdout.lines[0])

	// lock fee credit record
	stdout, err = execFeesCommand(logF, homedir, "lock -k 1")
	require.NoError(t, err)
	require.Equal(t, "Fee credit record locked successfully.", stdout.lines[0])

	// verify fee credit bill locked
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 0.999'999'97 (manually locked by user)"), stdout.lines[1])

	// unlock fee credit record
	stdout, err = execFeesCommand(logF, homedir, "unlock -k 1")
	require.NoError(t, err)
	require.Equal(t, "Fee credit record unlocked successfully.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, "Account #1 0.999'999'96", stdout.lines[1])
}

func execFeesCommand(logF LoggerFactory, homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(logF, homeDir, " fees "+command)
}

// setupMoneyInfraAndWallet creates wallet and starts money partition and wallet backend with initial bill belonging
// to the wallet. Returns wallet homedir and reference to money partition object.
func setupMoneyInfraAndWallet(t *testing.T, otherPartitions []*testpartition.NodePartition) (string, *testpartition.AlphabillNetwork) {
	// create wallet
	am, homedir := createNewWallet(t)
	defer am.Close()
	accountKey, err := am.GetAccountKey(0)
	require.NoError(t, err)
	initialBill := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: templates.NewP2pkh256BytesFromKey(accountKey.PubKey),
	}

	// start money partition
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	nodePartitions := []*testpartition.NodePartition{moneyPartition}
	nodePartitions = append(nodePartitions, otherPartitions...)
	abNet := startAlphabill(t, nodePartitions)

	startPartitionRPCServers(t, moneyPartition)
	startMoneyBackend(t, moneyPartition, initialBill)

	return homedir, abNet
}

func startMoneyBackend(t *testing.T, moneyPart *testpartition.NodePartition, initialBill *money.InitialBill) (string, *moneyclient.MoneyBackendClient) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := backend.Run(ctx,
			&backend.Config{
				ABMoneySystemIdentifier: money.DefaultSystemIdentifier,
				AlphabillUrl:            moneyPart.Nodes[0].AddrGRPC,
				ServerAddr:              defaultAlphabillApiURL, // TODO move to random port
				DbFile:                  filepath.Join(t.TempDir(), backend.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: backend.InitialBill{
					Id:        initialBill.ID,
					Value:     initialBill.Value,
					Predicate: initialBill.Owner,
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR, defaultTokenSDR},
				Logger:                   logger.New(t),
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	restClient, err := moneyclient.New(defaultAlphabillApiURL)
	require.NoError(t, err)

	// wait for backend to start
	require.Eventually(t, func() bool {
		rn, err := restClient.GetRoundNumber(ctx)
		return err == nil && rn > 0
	}, test.WaitDuration, test.WaitTick)

	return defaultAlphabillApiURL, restClient
}

func getFirstBillValue(t *testing.T, homedir string) string {
	// Account #1
	// #1 0x0000000000000000000000000000000000000000000000000000000000000001 9'999'999'849.999'999'91
	stdout := execWalletCmd(t, logger.LoggerBuilder(t), homedir, "bills list")
	require.Len(t, stdout.lines, 2)
	parts := strings.Split(stdout.lines[1], " ") // split by whitespace, should have 3 parts
	require.Len(t, parts, 3)
	return parts[2] // return last part i.e. the bill value
}
