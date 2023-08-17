package cmd

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	tokenswallet "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	tokensclient "github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
	vdwallet "github.com/alphabill-org/alphabill/pkg/wallet/vd"
)

const (
	apiUsage                   = "wallet backend API URL"
	partitionCmdName           = "partition"
	partitionBackendUrlCmdName = "partition-backend-url"
)

// newWalletFeesCmd creates a new cobra command for the wallet fees component.
func newWalletFeesCmd(config *walletConfig) *cobra.Command {
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
	cmd.AddCommand(listFeesCmd(config, cliConfig))
	cmd.AddCommand(addFeeCreditCmd(config, cliConfig))
	cmd.AddCommand(reclaimFeeCreditCmd(config, cliConfig))

	cmd.PersistentFlags().VarP(&cliConfig.partitionType, partitionCmdName, "n", "partition name for which to manage fees [money|tokens|vd]")
	cmd.PersistentFlags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, apiUsage)

	usage := fmt.Sprintf("partition backend url for which to manage fees (default: [%s|%s] based on --partition flag)", defaultAlphabillApiURL, defaultTokensBackendApiURL)
	cmd.PersistentFlags().StringVarP(&cliConfig.partitionBackendURL, partitionBackendUrlCmdName, "m", "", usage)
	return cmd
}

func addFeeCreditCmd(config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to add the fee credit")
	cmd.Flags().StringP(amountCmdName, "v", "1", "specifies how much fee credit to create in ALPHA")
	return cmd
}

func addFeeCreditCmdExec(cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
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
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(c, am, unitLocker, moneyBackendURL, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer fm.Close()

	return addFees(cmd.Context(), accountNumber, amountString, c, fm)
}

func listFeesCmd(config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listFeesCmdExec(cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account fee bills to list (default: all accounts)")
	return cmd
}

func listFeesCmdExec(cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(c, am, unitLocker, moneyBackendURL, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer fm.Close()

	return listFees(cmd.Context(), accountNumber, am, c, fm)
}

func reclaimFeeCreditCmd(config *walletConfig, c *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(cmd, config, c)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	return cmd
}

func reclaimFeeCreditCmdExec(cmd *cobra.Command, config *walletConfig, c *cliConf) error {
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(c, am, unitLocker, moneyBackendURL, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer fm.Close()

	return reclaimFees(cmd.Context(), accountNumber, c, fm)
}

type FeeCreditManager interface {
	GetFeeCredit(ctx context.Context, cmd fees.GetFeeCreditCmd) (*wallet.Bill, error)
	AddFeeCredit(ctx context.Context, cmd fees.AddFeeCmd) (*fees.AddFeeCmdResponse, error)
	ReclaimFeeCredit(ctx context.Context, cmd fees.ReclaimFeeCmd) (*fees.ReclaimFeeCmdResponse, error)
	Close()
}

func listFees(ctx context.Context, accountNumber uint64, am account.Manager, c *cliConf, w FeeCreditManager) error {
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		consoleWriter.Println("Partition: " + c.partitionType)
		for accountIndex := range pubKeys {
			fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: uint64(accountIndex)})
			if err != nil {
				return err
			}
			accNum := accountIndex + 1
			amountString := amountToString(fcb.GetValue(), 8)
			consoleWriter.Println(fmt.Sprintf("Account #%d %s", accNum, amountString))
		}
	} else {
		accountIndex := accountNumber - 1
		fcb, err := w.GetFeeCredit(ctx, fees.GetFeeCreditCmd{AccountIndex: accountIndex})
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
	singleFee := proofs.AddFC.TxRecord.ServerMetadata.ActualFee / 2 // For the user we display transFC and addFC fees individually
	if proofs.TransferFC != nil {
		consoleWriter.Println("Paid", amountToString(singleFee, 8), "fee for transferFC transaction from wallet balance.")
	} else {
		consoleWriter.Println("Used previously locked unit to create fee credit.")
	}
	consoleWriter.Println("Paid", amountToString(singleFee, 8), "fee for addFC transaction from fee credit balance.")
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
	if proofs.CloseFC != nil {
		consoleWriter.Println("Paid", amountToString(proofs.CloseFC.TxRecord.ServerMetadata.ActualFee, 8), "fee for closeFC transaction from fee credit balance.")
	} else {
		consoleWriter.Println("Used previously closed unit to reclaim fee credit.")
	}
	consoleWriter.Println("Paid", amountToString(proofs.ReclaimFC.TxRecord.ServerMetadata.ActualFee, 8), "fee for reclaimFC transaction from wallet balance.")
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
	case tokensType:
		return defaultTokensBackendApiURL
	case vdType:
		return defaultVDNodeURL
	default:
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}

// Creates a fees.FeeManager that needs to be closed with the Close() method.
// Does not close the account.Manager passed as an argument.
func getFeeCreditManager(c *cliConf, am account.Manager, unitLocker *unitlock.UnitLocker, moneyBackendURL, walletHomeDir string) (FeeCreditManager, error) {
	moneySystemID := money.DefaultSystemIdentifier
	moneyBackendClient, err := moneyclient.New(moneyBackendURL)
	if err != nil {
		return nil, err
	}
	moneyTxPublisher := moneywallet.NewTxPublisher(moneyBackendClient)

	if c.partitionType == moneyType {
		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
		), nil
	} else if c.partitionType == tokensType {
		backendURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, err
		}
		tokenBackendClient := tokensclient.New(*backendURL)
		tokenTxPublisher := tokenswallet.NewTxPublisher(tokenBackendClient)

		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			tokens.DefaultSystemIdentifier,
			tokenTxPublisher,
			tokenBackendClient,
		), nil
	} else if c.partitionType == vdType {
		vdClient, err := vdwallet.New(&vdwallet.VDClientConfig{
			VDNodeURL:     c.getPartitionBackendURL(),
			WalletHomeDir: walletHomeDir,
		})
		if err != nil {
			return nil, err
		}

		vdTxPublisher := vdwallet.NewTxPublisher(vdClient)

		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			vd.DefaultSystemIdentifier,
			vdTxPublisher,
			vdClient,
		), nil
	} else {
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}
