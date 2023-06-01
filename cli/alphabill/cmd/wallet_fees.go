package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	ttxs "github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	tokenclient "github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/spf13/cobra"
)

const (
	apiUsage                   = "wallet backend API URL"
	partitionCmdName           = "partition"
	partitionBackendUrlCmdName = "partition-backend-url"
)

// newWalletFeesCmd creates a new cobra command for the wallet fees component.
func newWalletFeesCmd(ctx context.Context, config *walletConfig) *cobra.Command {
	var cliConfig = &cliConf{
		partitionType: moneyType, // shows default value in help context
	}
	var cmd = &cobra.Command{
		Use:   "fees",
		Short: "cli for managing alphabill wallet fees",
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(listFeesCmd(ctx, config, cliConfig))
	cmd.AddCommand(addFeeCreditCmd(ctx, config, cliConfig))
	cmd.AddCommand(reclaimFeeCreditCmd(ctx, config, cliConfig))

	cmd.PersistentFlags().VarP(&cliConfig.partitionType, partitionCmdName, "n", "partition name for which to manage fees [money|token]")
	cmd.PersistentFlags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, apiUsage)

	usage := fmt.Sprintf("partition backend url for which to manage fees (default: [%s|%s] based on --partition flag)", defaultAlphabillApiURL, defaultTokensBackendApiURL)
	cmd.PersistentFlags().StringVarP(&cliConfig.partitionBackendURL, partitionBackendUrlCmdName, "m", "", usage)
	return cmd
}

func addFeeCreditCmd(ctx context.Context, config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(ctx, cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to add the fee credit")
	cmd.Flags().StringP(amountCmdName, "v", "1", "specifies how much fee credit to create in ALPHA")
	return cmd
}

func addFeeCreditCmdExec(ctx context.Context, cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	apiURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	amountString, err := cmd.Flags().GetString(amountCmdName)
	if err != nil {
		return err
	}
	moneyBackendClient, err := moneyclient.New(apiURL)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := getFeeCreditManager(c, am, moneyBackendClient)
	if err != nil {
		return err
	}
	return addFees(ctx, accountNumber, amountString, c, w)
}

func listFeesCmd(ctx context.Context, config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listFeesCmdExec(ctx, cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account fee bills to list (default: all accounts)")
	return cmd
}

func listFeesCmdExec(ctx context.Context, cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	moneyBackendClient, err := moneyclient.New(moneyBackendURL)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := getFeeCreditManager(c, am, moneyBackendClient)
	if err != nil {
		return err
	}
	return listFees(ctx, accountNumber, am, c, w)
}

func reclaimFeeCreditCmd(ctx context.Context, config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(ctx, cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	return cmd
}

func reclaimFeeCreditCmdExec(ctx context.Context, cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	moneyBackendApiURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	moneyBackendClient, err := moneyclient.New(moneyBackendApiURL)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	w, err := getFeeCreditManager(c, am, moneyBackendClient)
	if err != nil {
		return err
	}
	return reclaimFees(ctx, accountNumber, c, w)
}

type FeeCreditManager interface {
	GetFeeCreditBill(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error)
	AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) ([]*wallet.Proof, error)
	ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) ([]*wallet.Proof, error)
}

func listFees(ctx context.Context, accountNumber uint64, am account.Manager, c *cliConf, w FeeCreditManager) error {
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		consoleWriter.Println("Partition: " + c.partitionType)
		for accountIndex := range pubKeys {
			fcb, err := w.GetFeeCreditBill(ctx, fees.GetFeeCreditCmd{AccountIndex: uint64(accountIndex)})
			if err != nil {
				return err
			}
			accNum := accountIndex + 1
			amountString := amountToString(fcb.GetValue(), 8)
			consoleWriter.Println(fmt.Sprintf("Account #%d %s", accNum, amountString))
		}
	} else {
		accountIndex := accountNumber - 1
		fcb, err := w.GetFeeCreditBill(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
		if err != nil {
			return err
		}
		amountString := amountToString(fcb.GetValue(), 8)
		consoleWriter.Println("Partition: " + c.partitionType)
		consoleWriter.Println(fmt.Sprintf("Account #%d %s", accountNumber, amountString))
	}
	return nil
}

func addFees(ctx context.Context, accountNumber uint64, amountString string, c *cliConf, w FeeCreditManager) error {
	amount, err := stringToAmount(amountString, 8)
	if err != nil {
		return err
	}
	proofs, err := w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       amount,
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		return err
	}
	consoleWriter.Println("Successfully created", amountString, "fee credits on", c.partitionType, "partition.")
	consoleWriter.Println("Paid", amountToString(proofs[0].TxRecord.ServerMetadata.ActualFee, 8), "fee for transferFC transaction from wallet balance.")
	consoleWriter.Println("Paid", amountToString(proofs[1].TxRecord.ServerMetadata.ActualFee, 8), "fee for addFC transaction from fee credit balance.")
	return nil
}

func reclaimFees(ctx context.Context, accountNumber uint64, c *cliConf, w FeeCreditManager) error {
	proofs, err := w.ReclaimFeeCredit(ctx, fees.ReclaimFeeCmd{
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		return err
	}
	consoleWriter.Println("Successfully reclaimed fee credits on", c.partitionType, "partition.")
	consoleWriter.Println("Paid", amountToString(proofs[0].TxRecord.ServerMetadata.ActualFee, 8), "fee for closeFC transaction from fee credit balance.")
	consoleWriter.Println("Paid", amountToString(proofs[1].TxRecord.ServerMetadata.ActualFee, 8), "fee for reclaimFC transaction from wallet balance.")
	return nil
}

type cliConf struct {
	partitionType       partitionType
	partitionBackendURL string
}

func (c *cliConf) parsePartitionBackendURL() (*url.URL, error) {
	backendURL := c.getPartitionBackendURL()
	if !strings.HasPrefix(backendURL, "http://") && !strings.HasPrefix(backendURL, "https://") {
		backendURL = "http://" + backendURL
	}
	return url.Parse(backendURL)
}

func (c *cliConf) getPartitionBackendURL() string {
	if c.partitionBackendURL != "" {
		return c.partitionBackendURL
	}
	switch c.partitionType {
	case moneyType:
		return defaultAlphabillApiURL
	case tokenType:
		return defaultTokensBackendApiURL
	default:
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}

func getFeeCreditManager(c *cliConf, am account.Manager, moneyClient *moneyclient.MoneyBackendClient) (FeeCreditManager, error) {
	moneySystemID := []byte{0, 0, 0, 0}
	moneyTxPublisher := money.NewTxPublisher(moneyClient)
	if c.partitionType == moneyType {
		return money.NewFeeManager(am, moneySystemID, moneyClient), nil
	} else if c.partitionType == tokenType {
		backendURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, fmt.Errorf("failed to parse --%s", partitionBackendUrlCmdName)
		}
		tokenBackendClient := tokenclient.New(*backendURL)
		tokenTxPublisher := tokens.NewTxPublisher(tokenBackendClient)
		return fees.NewFeeManager(am, moneySystemID, moneyTxPublisher, moneyClient, ttxs.DefaultTokenTxSystemIdentifier, tokenTxPublisher, tokenBackendClient), nil
	} else {
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}
