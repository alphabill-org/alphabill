package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/script"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

func TestWalletFeesCmds(t *testing.T) {
	homedir, _ := setupInfraAndWallet(t)

	// list fees
	stdout, err := execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Account #1 0.00000000", stdout.lines[0])

	// add fee credits
	amount := uint64(150)
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits.", amount), stdout.lines[0])
	time.Sleep(2 * time.Second) // TODO waitForConf should use backend and not node for tx confirmations

	// verify fee credits
	expectedFees := amount*1e8 - 1
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[0])

	// add more fee credits
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits.", amount), stdout.lines[0])
	time.Sleep(2 * time.Second) // TODO waitForConf should use backend and not node for tx confirmations

	// verify fee credits
	expectedFees = amount*2*1e8 - 2
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Account #1 %s", amountToString(expectedFees, 8)), stdout.lines[0])

	// reclaim fees
	stdout, err = execFeesCommand(homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Account #1 0.00000000", stdout.lines[0])
}

func execFeesCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " fees "+command)
}

// setupInfraAndWallet starts money partiton and wallet backend and sends initial bill to wallet.
// Returns wallet homedir and reference to partition object.
func setupInfraAndWallet(t *testing.T) (string, *testpartition.AlphabillPartition) {
	initialBill := &moneytx.InitialBill{
		ID:    uint256.NewInt(1),
		Value: 1e18,
		Owner: script.PredicateAlwaysTrue(),
	}
	network := startAlphabillPartition(t, initialBill)
	startRPCServer(t, network, defaultAlphabillNodeURL)

	// start wallet backend
	ctx, cancelFunc := context.WithCancel(context.Background())
	t.Cleanup(cancelFunc)
	go func() {
		err := money.CreateAndRun(ctx,
			&money.Config{
				ABMoneySystemIdentifier: []byte{0, 0, 0, 0},
				AlphabillUrl:            defaultAlphabillNodeURL, // TODO move to random port
				ServerAddr:              defaultAlphabillApiURL,  // TODO move to random port
				DbFile:                  filepath.Join(t.TempDir(), money.BoltBillStoreFileName),
				ListBillsPageLimit:      100,
				InitialBill: money.InitialBill{
					Id:        util.Uint256ToBytes(initialBill.ID),
					Value:     initialBill.Value,
					Predicate: script.PredicateAlwaysTrue(),
				},
			})
		require.ErrorIs(t, err, context.Canceled)
	}()

	// create wallet
	wlog.InitStdoutLogger(wlog.DEBUG)
	homedir := createNewTestWallet(t)

	stdout := execWalletCmd(t, homedir, "get-pubkeys")
	require.Len(t, stdout.lines, 1)
	pk, _ := strings.CutPrefix(stdout.lines[0], "#1 ")

	// transfer initial bill to wallet pubkey
	spendInitialBillWithFeeCredits(t, network, initialBill, pk)

	// wait for initial bill tx
	waitForBalanceCLI(t, homedir, defaultAlphabillApiURL, initialBill.Value-3, 0) // initial bill minus txfees

	return homedir, network
}
