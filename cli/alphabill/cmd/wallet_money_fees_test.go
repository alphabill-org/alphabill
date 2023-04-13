package cmd

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalletFeesCmds(t *testing.T) {
	t.SkipNow() // TODO add fee credit support to "new" cli wallet
	homedir, _ := setupInfra(t)

	// list fees
	stdout, err := execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Account #1 0", stdout.lines[0])

	// add fee credits
	amount := 150
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits.", amount), stdout.lines[0])

	// add more fee credits
	stdout, err = execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Successfully created %d fee credits.", amount), stdout.lines[0])

	// list fees
	expectedFees := amount*2 - 2
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Account #1 %d", expectedFees), stdout.lines[0])

	// reclaim fees
	stdout, err = execFeesCommand(homedir, "reclaim")
	require.NoError(t, err)
	require.Equal(t, "Successfully reclaimed fee credits.", stdout.lines[0])

	// list fees
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, "Account #1 0", stdout.lines[0])
}

/*
Test that fee transactions are not processed multiple times when syncing wallet after sending txs with wait-for-conf
Test scenario:
start network and rpc server and send initial bill to wallet
wallet adds fee credit with multiple transactions
wallet runs sync
wallet balance and fee credit balance remain the same as before the sync
*/
func TestSyncDoesNotOverwritePreviouslyProcessedTxs(t *testing.T) {
	t.SkipNow() // TODO add fee credit support to "new" cli wallet
	homedir, _ := setupInfra(t)

	// add fee credits
	for i := 0; i < 3; i++ {
		amount := 11
		stdout, err := execFeesCommand(homedir, fmt.Sprintf("add --amount=%d", amount))
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("Successfully created %d fee credits.", amount), stdout.lines[0])
	}

	// list fees
	expectedFees := 30
	stdout, err := execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Account #1 %d", expectedFees), stdout.lines[0])

	// verify wallet balance
	verifyTotalBalance(t, homedir, 9961)

	// sync wallet (reprocess blocks with fee txs)
	stdout = execWalletCmd(t, homedir, "sync")
	verifyStdout(t, stdout, "Wallet synchronized successfully.")

	// verify fee credits remain the same
	stdout, err = execFeesCommand(homedir, "list")
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("Account #1 %d", expectedFees), stdout.lines[0])
}

func execFeesCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " fees "+command)
}
