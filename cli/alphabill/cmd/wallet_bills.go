package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newWalletBillsCmd creates a new cobra command for the wallet bills component.
func newWalletBillsCmd(config *walletConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "bills",
		Short: "cli for managing alphabill wallet bills and proofs",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(listCmd(config))
	return cmd
}

func listCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execListCmd(cmd, config)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies which account bills to list")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}

	bills, err := w.GetBills(accountNumber - 1)
	if err != nil {
		return err
	}
	if len(bills) == 0 {
		consoleWriter.Println("Wallet is empty.")
		return nil
	}
	for i, b := range bills {
		consoleWriter.Println(fmt.Sprintf("#%d 0x%X %d", i+1, b.Id.Bytes32(), b.Value))
	}
	return nil
}
