package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/internal/util"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var defaultTokenSDR = &genesis.SystemDescriptionRecord{
	SystemIdentifier: tokens.DefaultSystemIdentifier,
	T2Timeout:        defaultT2Timeout,
	FeeCreditBill: &genesis.FeeCreditBill{
		UnitId:         util.Uint256ToBytes(uint256.NewInt(4)),
		OwnerPredicate: script.PredicateAlwaysTrue(),
	},
}

var defaultVDSDR = &genesis.SystemDescriptionRecord{
	SystemIdentifier: vd.DefaultSystemIdentifier,
	T2Timeout:        defaultT2Timeout,
	FeeCreditBill: &genesis.FeeCreditBill{
		UnitId:         util.Uint256ToBytes(uint256.NewInt(2)),
		OwnerPredicate: script.PredicateAlwaysTrue(),
	},
}

func TestWalletFeesCmds_MoneyPartition(t *testing.T) {
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{})

	// list fees
	stdout, err := execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 1
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// add more fee credits
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees = amount*2*1e8 - 2
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim fees
	stdout, err = execFeesCommand(homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on money partition.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on money partition.", amount), stdout.lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 1
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Partition: money", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])
}

func TestWalletFeesCmds_TokenPartition(t *testing.T) {
	// start money partition and create wallet with token partition as well
	tokensPartition := createTokensPartition(t)
	homedir, _ := setupMoneyInfraAndWallet(t, []*testpartition.NodePartition{tokensPartition})

	// start token partition
	startPartitionRPCServers(t, tokensPartition)

	tokenBackendURL, _, _ := startTokensBackend(t, tokensPartition.Nodes[0].AddrGRPC)
	args := fmt.Sprintf("--partition=tokens -r %s -m %s", defaultAlphabillApiURL, tokenBackendURL)

	// list fees on token partition
	stdout, err := execFeesCommand(homedir, "list "+args)
	require.NoError(t, err)

	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify fee credits
	expectedFees := amount*1e8 - 1
	stdout, err = execFeesCommand(homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// add more fee credits to token partition
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify fee credits to token partition
	expectedFees = amount*2*1e8 - 2
	stdout, err = execFeesCommand(homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])

	// reclaim fees
	stdout, err = execFeesCommand(homedir, "reclaim "+args)
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits on tokens partition.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, "Account #1 0.000'000'00", stdout.lines[1])

	// add more fees after reclaiming
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d %s", amount, args))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits on tokens partition.", amount), stdout.lines[0])

	// verify list fees
	expectedFees = amount*1e8 - 1
	stdout, err = execFeesCommand(homedir, "list "+args)
	require.NoError(t, err)
	require.Equal(t, "Partition: tokens", stdout.lines[0])
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[1])
}

func execFeesCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " fees "+command)
}

// setupMoneyInfraAndWallet starts money partition and wallet backend and sends initial bill to wallet.
// Returns wallet homedir and reference to money partition object.
func setupMoneyInfraAndWallet(t *testing.T, otherPartitions []*testpartition.NodePartition) (string, *testpartition.AlphabillNetwork) {
	initialBill := &money.InitialBill{
		ID:    util.Uint256ToBytes(uint256.NewInt(1)),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	moneyPartition := createMoneyPartition(t, initialBill)
	nodePartitions := []*testpartition.NodePartition{moneyPartition}
	nodePartitions = append(nodePartitions, otherPartitions...)
	abNet := startAlphabill(t, nodePartitions)

	startPartitionRPCServers(t, moneyPartition)

	startMoneyBackend(t, moneyPartition, initialBill)

	// create wallet
	wlog.InitStdoutLogger(wlog.DEBUG)
	homedir := createNewTestWallet(t)

	stdout := execWalletCmd(t, homedir, "get-pubkeys")
	require.Len(t, stdout.lines, 1)
	pk, _ := strings.CutPrefix(stdout.lines[0], "#1 ")

	// transfer initial bill to wallet pubkey
	spendInitialBillWithFeeCredits(t, abNet, initialBill, pk)

	// wait for initial bill tx
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, initialBill.Value-3, 0) // initial bill minus txfees

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
					Predicate: script.PredicateAlwaysTrue(),
				},
				SystemDescriptionRecords: []*genesis.SystemDescriptionRecord{defaultMoneySDR, defaultVDSDR, defaultTokenSDR},
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	restClient, err := moneyclient.New(defaultAlphabillApiURL)
	require.NoError(t, err)

	return defaultAlphabillApiURL, restClient
}
