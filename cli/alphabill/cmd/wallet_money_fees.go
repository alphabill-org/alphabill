package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/pkg/client"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
)

// newWalletFeesCmd creates a new cobra command for the wallet fees component.
func newWalletFeesCmd(ctx context.Context, config *walletConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "fees",
		Short: "cli for managing alphabill wallet fees",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(listFeesCmd(config))
	cmd.AddCommand(addFeeCreditCmd(ctx, config))
	cmd.AddCommand(reclaimFeeCreditCmd(ctx, config))
	return cmd
}

func addFeeCreditCmd(ctx context.Context, config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(ctx, cmd, config)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to add the fee credit")
	cmd.Flags().Uint64P(amountCmdName, "v", 100, "specifies how much fee credit to create")
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", alphabillNodeURLCmdName, "node url")
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", alphabillApiURLCmdName, "alphabill API uri to connect to")
	return cmd
}

func addFeeCreditCmdExec(ctx context.Context, cmd *cobra.Command, config *walletConfig) error {
	nodeUrl, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return err
	}
	apiUrl, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}

	restClient, err := moneyclient.NewClient(apiUrl)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{Uri: nodeUrl}, am, restClient)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	amount, err := cmd.Flags().GetUint64(amountCmdName)
	if err != nil {
		return err
	}
	_, err = w.AddFeeCredit(ctx, money.AddFeeCmd{
		Amount:       amount,
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		return err
	}
	consoleWriter.Println("Successfully created", amount, "fee credits.")
	return nil
}

func listFeesCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listFeesCmdExec(cmd, config)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account fee bills to list (default: all accounts)")
	return cmd
}

func listFeesCmdExec(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}

	nodeUrl, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return err
	}
	apiUrl, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.NewClient(apiUrl)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{Uri: nodeUrl}, am, restClient)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		for accountIndex := range pubKeys {
			fcb, err := w.GetFeeCreditBill(uint64(accountIndex))
			if err != nil {
				return err
			}
			accNum := accountIndex + 1
			consoleWriter.Println(fmt.Sprintf("Account #%d %d", accNum, getValue(fcb)))
		}
	} else {
		accountIndex := accountNumber - 1
		fcb, err := w.GetFeeCreditBill(accountIndex)
		if err != nil {
			return err
		}
		consoleWriter.Println(fmt.Sprintf("Account #%d %d", accountNumber, getValue(fcb)))
	}
	return nil
}

func reclaimFeeCreditCmd(ctx context.Context, config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(ctx, cmd, config)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", alphabillNodeURLCmdName, "node url")
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", alphabillApiURLCmdName, "alphabill API uri to connect to")
	return cmd
}

func reclaimFeeCreditCmdExec(ctx context.Context, cmd *cobra.Command, config *walletConfig) error {
	nodeUrl, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return err
	}
	apiUrl, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.NewClient(apiUrl)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{Uri: nodeUrl}, am, restClient)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	_, err = w.ReclaimFeeCredit(ctx, money.ReclaimFeeCmd{
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		return err
	}
	consoleWriter.Println("Successfully reclaimed fee credits.")
	return nil
}

func getValue(b *money.Bill) uint64 {
	if b != nil {
		return b.Value
	}
	return 0
}
