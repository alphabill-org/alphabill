package cmd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	evmwallet "github.com/alphabill-org/alphabill/pkg/wallet/evm"
	evmclient "github.com/alphabill-org/alphabill/pkg/wallet/evm/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/fees"
	moneywallet "github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	tokenswallet "github.com/alphabill-org/alphabill/pkg/wallet/tokens"
	tokensclient "github.com/alphabill-org/alphabill/pkg/wallet/tokens/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/unitlock"
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

	cmd.PersistentFlags().VarP(&cliConfig.partitionType, partitionCmdName, "n", "partition name for which to manage fees [money|tokens|evm]")
	cmd.PersistentFlags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, apiUsage)

	usage := fmt.Sprintf("partition backend url for which to manage fees (default: [%s|%s|%s] based on --partition flag)", defaultAlphabillApiURL, defaultTokensBackendApiURL, defaultEvmNodeRestURL)
	cmd.PersistentFlags().StringVarP(&cliConfig.partitionBackendURL, partitionBackendUrlCmdName, "m", "", usage)
	return cmd
}

func addFeeCreditCmd(walletConfig *walletConfig, cliConfig *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "adds fee credit to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return addFeeCreditCmdExec(cmd, walletConfig, cliConfig)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to add the fee credit")
	cmd.Flags().StringP(amountCmdName, "v", "1", "specifies how much fee credit to create in ALPHA")
	return cmd
}

func addFeeCreditCmdExec(cmd *cobra.Command, walletConfig *walletConfig, cliConfig *cliConf) error {
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

	am, err := loadExistingAccountManager(cmd, walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(cmd.Context(), cliConfig, am, unitLocker, moneyBackendURL, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	return addFees(cmd.Context(), accountNumber, amountString, cliConfig, fm)
}

func listFeesCmd(walletConfig *walletConfig, cliConfig *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return listFeesCmdExec(cmd, walletConfig, cliConfig)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account fee bills to list (default: all accounts)")
	return cmd
}

func listFeesCmdExec(cmd *cobra.Command, walletConfig *walletConfig, cliConfig *cliConf) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}

	am, err := loadExistingAccountManager(cmd, walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(cmd.Context(), cliConfig, am, unitLocker, moneyBackendURL, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	return listFees(cmd.Context(), accountNumber, am, cliConfig, fm)
}

func reclaimFeeCreditCmd(walletConfig *walletConfig, cliConfig *cliConf) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reclaim",
		Short: "reclaims fee credit of the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return reclaimFeeCreditCmdExec(cmd, walletConfig, cliConfig)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to reclaim the fee credit")
	return cmd
}

func reclaimFeeCreditCmdExec(cmd *cobra.Command, walletConfig *walletConfig, cliConfig *cliConf) error {
	moneyBackendURL, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}

	am, err := loadExistingAccountManager(cmd, walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(walletConfig.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	fm, err := getFeeCreditManager(cmd.Context(), cliConfig, am, unitLocker, moneyBackendURL, walletConfig.Base.Logger)
	if err != nil {
		return err
	}
	defer fm.Close()

	return reclaimFees(cmd.Context(), accountNumber, cliConfig, fm)
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
	addFeeCmdResponse, err := w.AddFeeCredit(ctx, fees.AddFeeCmd{
		Amount:       amount,
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("minimum fee credit amount to add is %s", amountToString(fees.MinimumFeeAmount, 8))
		}
		if errors.Is(err, fees.ErrInsufficientBalance) {
			return fmt.Errorf("insufficient balance for transaction. Bills smaller than the minimum amount (%s) are not counted", amountToString(fees.MinimumFeeAmount, 8))
		}
		return err
	}
	consoleWriter.Println("Successfully created", amountString, "fee credits on", c.partitionType, "partition.")
	if len(addFeeCmdResponse.TransferFC) > 0 {
		var feeSum uint64
		for _, proof := range addFeeCmdResponse.TransferFC {
			feeSum += proof.TxRecord.ServerMetadata.GetActualFee()
		}
		consoleWriter.Println("Paid", amountToString(feeSum, 8), "fee for transferFC transaction.")
	} else {
		consoleWriter.Println("Used previously locked unit to create fee credit.")
	}
	var feeSum uint64
	for _, proof := range addFeeCmdResponse.AddFC {
		feeSum += proof.TxRecord.ServerMetadata.GetActualFee()
	}
	consoleWriter.Println("Paid", amountToString(feeSum, 8), "fee for addFC transaction.")
	return nil
}

func reclaimFees(ctx context.Context, accountNumber uint64, c *cliConf, w FeeCreditManager) error {
	proofs, err := w.ReclaimFeeCredit(ctx, fees.ReclaimFeeCmd{
		AccountIndex: accountNumber - 1,
	})
	if err != nil {
		if errors.Is(err, fees.ErrMinimumFeeAmount) {
			return fmt.Errorf("insufficient fee credit balance. Minimum amount is %s", amountToString(fees.MinimumFeeAmount, 8))
		}
		return err
	}
	consoleWriter.Println("Successfully reclaimed fee credits on", c.partitionType, "partition.")
	if proofs.CloseFC != nil {
		consoleWriter.Println("Paid", amountToString(proofs.CloseFC.TxRecord.ServerMetadata.ActualFee, 8), "fee for closeFC transaction.")
	} else {
		consoleWriter.Println("Used previously closed unit to reclaim fee credit.")
	}
	consoleWriter.Println("Paid", amountToString(proofs.ReclaimFC.TxRecord.ServerMetadata.ActualFee, 8), "fee for reclaimFC transaction.")
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
	case evmType:
		return defaultEvmNodeRestURL // evm does not use backend and instead talks to an actual evm node
	default:
		panic("invalid \"partition\" flag value: " + c.partitionType)
	}
}

// Creates a fees.FeeManager that needs to be closed with the Close() method.
// Does not close the account.Manager passed as an argument.
func getFeeCreditManager(ctx context.Context, c *cliConf, am account.Manager, unitLocker *unitlock.UnitLocker, moneyBackendURL string, log *slog.Logger) (FeeCreditManager, error) {
	moneyBackendClient, err := moneyclient.New(moneyBackendURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create money backend client: %w", err)
	}
	moneySystemInfo, err := moneyBackendClient.GetInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch money system info: %w", err)
	}
	moneyTypeVar := moneyType
	if !strings.HasPrefix(moneySystemInfo.Name, moneyTypeVar.String()) {
		return nil, fmt.Errorf("invalid money backend name: %s", moneySystemInfo.Name)
	}
	moneySystemID, err := hex.DecodeString(moneySystemInfo.SystemID)
	if err != nil {
		return nil, fmt.Errorf("failed to decode money system identifier hex: %w", err)
	}
	moneyTxPublisher := moneywallet.NewTxPublisher(moneyBackendClient, log)

	switch c.partitionType {
	case moneyType:
		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			moneywallet.FeeCreditRecordIDFormPublicKey,
			log,
		), nil
	case tokensType:
		backendURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, fmt.Errorf("failed to parse partition backend url: %w", err)
		}
		tokenBackendClient := tokensclient.New(*backendURL)
		tokenTxPublisher := tokenswallet.NewTxPublisher(tokenBackendClient, log)
		tokenInfo, err := tokenBackendClient.GetInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch tokens system info: %w", err)
		}
		tokenTypeVar := tokensType
		if !strings.HasPrefix(tokenInfo.Name, tokenTypeVar.String()) {
			return nil, fmt.Errorf("invalid tokens backend name: %s", tokenInfo.Name)
		}
		tokenSystemID, err := hex.DecodeString(tokenInfo.SystemID)
		if err != nil {
			return nil, fmt.Errorf("failed to decode tokens system identifier hex: %w", err)
		}
		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			tokenSystemID,
			tokenTxPublisher,
			tokenBackendClient,
			tokenswallet.FeeCreditRecordIDFromPublicKey,
			log,
		), nil
	case evmType:
		evmNodeURL, err := c.parsePartitionBackendURL()
		if err != nil {
			return nil, err
		}
		evmClient := evmclient.New(*evmNodeURL)
		evmTxPublisher := evmwallet.NewTxPublisher(evmClient)
		evmInfo, err := evmClient.GetInfo(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch evm system info: %w", err)
		}
		evmTypeVar := evmType
		if !strings.HasPrefix(evmInfo.Name, evmTypeVar.String()) {
			return nil, fmt.Errorf("invalid evm partition name: %s", evmInfo.Name)
		}
		evmSystemID, err := hex.DecodeString(evmInfo.SystemID)
		if err != nil {
			return nil, fmt.Errorf("failed to decode evm system identifier hex: %w", err)
		}
		return fees.NewFeeManager(
			am,
			unitLocker,
			moneySystemID,
			moneyTxPublisher,
			moneyBackendClient,
			evmSystemID,
			evmTxPublisher,
			evmClient,
			evmwallet.FeeCreditRecordIDFromPublicKey,
			log,
		), nil
	default:
		panic(`invalid "partition" flag value: ` + c.partitionType)
	}
}
