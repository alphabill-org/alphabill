package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/validator/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/validator/internal/testutils/logger"
	testpartition "github.com/alphabill-org/alphabill/validator/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/validator/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/validator/pkg/wallet/money/backend"
	moneyclient "github.com/alphabill-org/alphabill/validator/pkg/wallet/money/backend/client"
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
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{}, logF)

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
	expectedFees = amount*2*1e8 - 4
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
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{tokensPartition}, logF)

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
	expectedFees = amount*2*1e8 - 4
	stdout, err = execFeesCommand(logF, homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim fees
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
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{}, logF)

	// try to add invalid fee amount
	_, err := execFeesCommand(logF, homedir, "add --amount=0.00000002")
	require.Errorf(t, err, "minimum fee credit amount to add is %d", amountToString(fees.MinimumFeeAmount, 8))

	// add minimum fee amount
	stdout, err := execFeesCommand(logF, homedir, "add --amount=0.00000003")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000003 fee credits on money partition.", stdout.lines[0])

	// verify fee credit is below minimum
	expectedFees := uint64(1)
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim with invalid amount
	stdout, err = execFeesCommand(logF, homedir, "reclaim")
	require.Errorf(t, err, "insufficient fee credit balance. Minimum amount is %d", amountToString(fees.MinimumFeeAmount, 8))
	require.Empty(t, stdout.lines)

	// add more fee credit
	stdout, err = execFeesCommand(logF, homedir, "add --amount=0.00000004")
	require.NoError(t, err)
	require.Equal(t, "Successfully created 0.00000004 fee credits on money partition.", stdout.lines[0])

	// verify fee credit is valid for reclaim
	expectedFees = uint64(3)
	stdout, err = execFeesCommand(logF, homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// now we have enough credit to reclaim
	stdout, err = execFeesCommand(logF, homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.lines[0])
}

func execFeesCommand(logF LoggerFactory, homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(logF, homeDir, " fees "+command)
}

// setupMoneyInfraAndWallet starts money partition and wallet backend and sends initial bill to wallet.
// Returns wallet homedir and reference to money partition object.
func setupMoneyInfraAndWallet(t *testing.T, otherPartitions []*testpartition.NodePartition, logF LoggerFactory) (string, *testpartition.AlphabillNetwork) {
	initialBill := &money.InitialBill{
		ID:    defaultInitialBillID,
		Value: 1e18,
		Owner: templates.AlwaysTrueBytes(),
	}
	moneyPartition := createMoneyPartition(t, initialBill, 1)
	nodePartitions := []*testpartition.NodePartition{moneyPartition}
	nodePartitions = append(nodePartitions, otherPartitions...)
	abNet := startAlphabill(t, nodePartitions)

	startPartitionRPCServers(t, moneyPartition)

	startMoneyBackend(t, moneyPartition, initialBill)

	// create wallet
	homedir := createNewTestWallet(t)

	stdout := execWalletCmd(t, logF, homedir, "get-pubkeys")
	require.Len(t, stdout.lines, 1)
	pk, _ := strings.CutPrefix(stdout.lines[0], "#1 ")
	pkBytes, _ := hexutil.Decode(pk)

	// transfer initial bill to wallet pubkey
	expectedValue := spendInitialBillWithFeeCredits(t, abNet, initialBill, pkBytes)

	// wait for initial bill tx
	waitForBalanceCLI(t, logF, homedir, defaultAlphabillApiURL, expectedValue, 0)

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
					Predicate: templates.AlwaysTrueBytes(),
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR, defaultTokenSDR},
				Logger:                   logger.New(t),
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	restClient, err := moneyclient.New(defaultAlphabillApiURL)
	require.NoError(t, err)

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
