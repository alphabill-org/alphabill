package cmd

import (
	"testing"
)

func TestWalletBillsListCmd(t *testing.T) {
	homedir := createNewTestWallet(t)
	stdout := execBillsCommand(t, homedir, "list")
	verifyStdout(t, stdout, "Wallet is empty.")

	// TODO add test that verifies list bills output after import proof is done.
}

func execBillsCommand(t *testing.T, homeDir, command string) *testConsoleWriter {
	return execCommand(t, homeDir, " bills "+command)
}
