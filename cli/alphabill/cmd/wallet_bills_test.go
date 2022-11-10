package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWalletBillsListCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout, err := execBillsCommand(homedir, "list")
	require.NoError(t, err)
	verifyStdout(t, stdout, "Wallet is empty.")

	// TODO add test that verifies list bills output after import proof is done.
}

func TestWalletBillsExportCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	_, err := execBillsCommand(homedir, "export --bill-id=00")
	require.ErrorContains(t, err, "bill does not exist")

	// TODO add test that verifies export bill output after import proof is done.
}

func execBillsCommand(homeDir, command string) (*testConsoleWriter, error) {
	return execCommand(homeDir, " bills "+command)
}
