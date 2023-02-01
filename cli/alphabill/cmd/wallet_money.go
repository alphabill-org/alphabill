package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
)

const (
	defaultAlphabillUri = "localhost:9543"
	passwordPromptUsage = "password (interactive from prompt)"
	passwordArgUsage    = "password (non-interactive from args)"

	alphabillUriCmdName   = "alphabill-uri"
	seedCmdName           = "seed"
	addressCmdName        = "address"
	amountCmdName         = "amount"
	passwordPromptCmdName = "password"
	passwordArgCmdName    = "pn"
	logFileCmdName        = "log-file"
	logLevelCmdName       = "log-level"
	walletLocationCmdName = "wallet-location"
	keyCmdName            = "key"
	waitForConfCmdName    = "wait-for-confirmation"
	totalCmdName          = "total"
	quietCmdName          = "quiet"
	showUnswappedCmdName  = "show-unswapped"
)

type walletConfig struct {
	Base          *baseConfiguration
	WalletHomeDir string
	LogLevel      string
	LogFile       string
}

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &walletConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "cli for managing alphabill wallet",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// initialize config so that baseConfig.HomeDir gets configured
			err := initializeConfig(cmd, baseConfig)
			if err != nil {
				return err
			}
			err = initWalletConfig(cmd, config)
			if err != nil {
				return err
			}
			return initWalletLogger(config)
		},
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand like create, sync, send etc")
		},
	}
	walletCmd.AddCommand(newWalletBillsCmd(config))
	walletCmd.AddCommand(createCmd(config))
	walletCmd.AddCommand(syncCmd(config))
	walletCmd.AddCommand(getBalanceCmd(config))
	walletCmd.AddCommand(getPubKeysCmd(config))
	walletCmd.AddCommand(sendCmd(ctx, config))
	walletCmd.AddCommand(collectDustCmd(config))
	walletCmd.AddCommand(addKeyCmd(config))
	walletCmd.AddCommand(tokenCmd(config))
	// add passwords flags for (encrypted)wallet
	walletCmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	walletCmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	walletCmd.PersistentFlags().StringVar(&config.LogFile, logFileCmdName, "", fmt.Sprintf("log file path (default output to stderr)"))
	walletCmd.PersistentFlags().StringVar(&config.LogLevel, logLevelCmdName, "INFO", fmt.Sprintf("logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)"))
	walletCmd.PersistentFlags().StringVarP(&config.WalletHomeDir, walletLocationCmdName, "l", "", fmt.Sprintf("wallet home directory (default $AB_HOME/wallet)"))
	return walletCmd
}

func createCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCreateCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(seedCmdName, "s", "", "mnemonic seed, the number of words should be 12, 15, 18, 21 or 24")
	return cmd
}

func execCreateCmd(cmd *cobra.Command, config *walletConfig) error {
	mnemonic, err := cmd.Flags().GetString(seedCmdName)
	if err != nil {
		return err
	}
	password, err := createPassphrase(cmd)
	if err != nil {
		return err
	}
	am, err := account.NewManager(config.WalletHomeDir, password, true)
	if err != nil {
		return err
	}
	c := money.WalletConfig{DbPath: config.WalletHomeDir}
	var w *money.Wallet
	consoleWriter.Println("Creating new wallet...")
	w, err = money.CreateNewWallet(am, mnemonic, c)
	if err != nil {
		return err
	}
	defer w.Shutdown()
	consoleWriter.Println("Wallet created successfully.")

	// print mnemonic if new wallet was created
	if mnemonic == "" {
		mnemonicSeed, err := w.GetAccountManager().GetMnemonic()
		if err != nil {
			return err
		}
		consoleWriter.Println("The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
		consoleWriter.Println("mnemonic key: " + mnemonicSeed)
	}
	return nil
}

func syncCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSyncCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	return cmd
}

func execSyncCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, uri)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	consoleWriter.Println("Starting wallet synchronization...")
	err = w.SyncToMaxBlockNumber(ctx)
	if err != nil {
		consoleWriter.Println("Failed to synchronize wallet: " + err.Error())
		return err
	}
	consoleWriter.Println("Wallet synchronized successfully.")
	return nil
}

func sendCmd(ctx context.Context, config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSendCmd(ctx, cmd, config)
		},
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	cmd.Flags().Uint64P(amountCmdName, "v", 0, "the amount to send to the receiver")
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	// use string instead of boolean as boolean requires equals sign between name and value e.g. w=[true|false]
	cmd.Flags().StringP(waitForConfCmdName, "w", "true", "waits for transaction confirmation on the blockchain, otherwise just broadcasts the transaction")
	cmd.Flags().StringP(outputPathCmdName, "o", "", "saves transaction proof(s) to given directory")
	err := cmd.MarkFlagRequired(addressCmdName)
	if err != nil {
		return nil
	}
	err = cmd.MarkFlagRequired(amountCmdName)
	if err != nil {
		return nil
	}
	return cmd
}

func execSendCmd(ctx context.Context, cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, uri)
	if err != nil {
		return err
	}
	defer w.Shutdown()
	pubKeyHex, err := cmd.Flags().GetString(addressCmdName)
	if err != nil {
		return err
	}
	pubKey, ok := pubKeyHexToBytes(pubKeyHex)
	if !ok {
		return errors.New("address in not in valid format")
	}
	amount, err := cmd.Flags().GetUint64(amountCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	waitForConfStr, err := cmd.Flags().GetString(waitForConfCmdName)
	if err != nil {
		return err
	}
	waitForConf, err := strconv.ParseBool(waitForConfStr)
	if err != nil {
		return err
	}
	outputPath, err := cmd.Flags().GetString(outputPathCmdName)
	if err != nil {
		return err
	}
	if outputPath != "" {
		if !waitForConf {
			return fmt.Errorf("cannot set %s to false and when %s is provided", waitForConfCmdName, outputPathCmdName)
		}
		if !strings.HasPrefix(outputPath, string(os.PathSeparator)) {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			outputPath = filepath.Join(cwd, outputPath)
		}
	}
	bills, err := w.Send(ctx, money.SendCmd{ReceiverPubKey: pubKey, Amount: amount, WaitForConfirmation: waitForConf, AccountIndex: accountNumber - 1})
	if err != nil {
		return err
	}
	if waitForConf {
		consoleWriter.Println("Successfully confirmed transaction(s)")
		if outputPath != "" {
			outputFile, err := writeBillsToFile(outputPath, bills...)
			if err != nil {
				return err
			}
			consoleWriter.Println("Transaction proof(s) saved to: " + outputFile)
		}
	} else {
		consoleWriter.Println("Successfully sent transaction(s)")
	}
	return nil
}

func getBalanceCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetBalanceCmd(cmd, config)
		},
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which key balance to query "+
		"(by default returns all key balances including total balance over all keys)")
	cmd.Flags().BoolP(totalCmdName, "t", false,
		"if specified shows only total balance over all accounts")
	cmd.Flags().BoolP(quietCmdName, "q", false, "hides info irrelevant for scripting, "+
		"e.g. account key numbers, can only be used together with key or total flag")
	cmd.Flags().BoolP(showUnswappedCmdName, "s", false, "includes unswapped dust bills in balance output")
	return cmd
}

func execGetBalanceCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	total, err := cmd.Flags().GetBool(totalCmdName)
	if err != nil {
		return err
	}
	quiet, err := cmd.Flags().GetBool(quietCmdName)
	if err != nil {
		return err
	}
	showUnswapped, err := cmd.Flags().GetBool(showUnswappedCmdName)
	if err != nil {
		return err
	}
	if !total && accountNumber == 0 {
		quiet = false // quiet is supposed to work only when total or key flag is provided
	}
	if accountNumber == 0 {
		sum := uint64(0)
		balances, err := w.GetBalances(money.GetBalanceCmd{CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		for accountIndex, accountBalance := range balances {
			sum += accountBalance
			if !total {
				consoleWriter.Println(fmt.Sprintf("#%d %d", accountIndex+1, accountBalance))
			}
		}
		if quiet {
			consoleWriter.Println(sum)
		} else {
			consoleWriter.Println(fmt.Sprintf("Total %d", sum))
		}
	} else {
		balance, err := w.GetBalance(money.GetBalanceCmd{AccountIndex: accountNumber - 1, CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		if quiet {
			consoleWriter.Println(balance)
		} else {
			consoleWriter.Println(fmt.Sprintf("#%d %d", accountNumber, balance))
		}
	}
	return nil
}

func getPubKeysCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-pubkeys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetPubKeysCmd(cmd, config)
		},
	}
	cmd.Flags().BoolP(quietCmdName, "q", false, "hides info irrelevant for scripting, e.g. account key numbers")
	return cmd
}

func execGetPubKeysCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	pubKeys, err := w.GetAccountManager().GetPublicKeys()
	if err != nil {
		return err
	}
	hideKeyNumber, _ := cmd.Flags().GetBool(quietCmdName)
	for accIdx, accPubKey := range pubKeys {
		if hideKeyNumber {
			consoleWriter.Println(fmt.Sprintf("%s", hexutil.Encode(accPubKey)))
		} else {
			consoleWriter.Println(fmt.Sprintf("#%d %s", accIdx+1, hexutil.Encode(accPubKey)))
		}
	}
	return nil
}

func collectDustCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "consolidates bills and synchronizes wallet",
		Long:  "consolidates all bills into a single bill and synchronizes wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	return cmd
}

func execCollectDust(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, uri)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	consoleWriter.Println("Starting dust collection, this may take a while...")
	// start dust collection by calling CollectDust (sending dc transfers) and Sync (waiting for dc transfers to confirm)
	// any error from CollectDust or Sync causes either goroutine to terminate
	// if collect dust returns without error we signal Sync to cancel manually

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		err := w.CollectDust(ctx)
		if err == nil {
			defer cancel() // signal Sync to cancel
		}
		return err
	})
	group.Go(func() error {
		return w.Sync(ctx)
	})
	err = group.Wait()
	if err != nil {
		consoleWriter.Println("Failed to collect dust: " + err.Error())
		return err
	}
	consoleWriter.Println("Dust collection finished successfully.")
	return nil
}

func addKeyCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-key",
		Short: "adds the next key in the series to the wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execAddKeyCmd(cmd, config)
		},
	}
	return cmd
}

func execAddKeyCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accIdx, accPubKey, err := w.AddAccount()
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Added key #%d %s", accIdx+1, hexutil.Encode(accPubKey)))
	return nil
}

func loadExistingWallet(cmd *cobra.Command, walletDir string, uri string) (*money.Wallet, error) {
	config := money.WalletConfig{
		DbPath:                walletDir,
		AlphabillClientConfig: client.AlphabillClientConfig{Uri: uri},
	}
	am, err := loadExistingAccountManager(cmd, walletDir)
	if err != nil {
		return nil, err
	}
	return money.LoadExistingWallet(config, am)
}

func loadExistingAccountManager(cmd *cobra.Command, walletDir string) (account.Manager, error) {
	pw := ""
	am, err := account.NewManager(walletDir, pw, false)
	if err != nil {
		return nil, err
	}
	isEncrypted, err := am.IsEncrypted()
	if err != nil {
		return nil, err
	}
	if isEncrypted {
		walletPass, err := getPassphrase(cmd, "Enter passphrase: ")
		if err != nil {
			return nil, err
		}
		pw = walletPass
	}
	am, err = account.NewManager(walletDir, pw, false)
	if err != nil {
		return nil, err
	}
	return am, nil
}

func initWalletConfig(cmd *cobra.Command, config *walletConfig) error {
	walletLocation, err := cmd.Flags().GetString(walletLocationCmdName)
	if err != nil {
		return err
	}
	if walletLocation != "" {
		config.WalletHomeDir = walletLocation
	} else {
		config.WalletHomeDir = filepath.Join(config.Base.HomeDir, "wallet")
	}
	return nil
}

func initWalletLogger(config *walletConfig) error {
	var logWriter io.Writer
	if config.LogFile != "" {
		// ensure intermediate directories exist
		err := os.MkdirAll(filepath.Dir(config.LogFile), 0700)
		if err != nil {
			return err
		}
		logFile, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // -rw-------
		if err != nil {
			return err
		}
		logWriter = logFile
	} else {
		logWriter = os.Stderr
	}
	logLevel := wlog.Levels[config.LogLevel]
	walletLogger, err := wlog.New(logLevel, logWriter)
	if err != nil {
		return err
	}
	wlog.SetLogger(walletLogger)
	return nil
}

func createPassphrase(cmd *cobra.Command) (string, error) {
	passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
	if err != nil {
		return "", err
	}
	if passwordFromArg != "" {
		return passwordFromArg, nil
	}
	passwordFlag, err := cmd.Flags().GetBool(passwordPromptCmdName)
	if err != nil {
		return "", err
	}
	if !passwordFlag {
		return "", nil
	}
	p1, err := readPassword("Create new passphrase: ")
	if err != nil {
		return "", err
	}
	p2, err := readPassword("Confirm passphrase: ")
	if err != nil {
		return "", err
	}
	if p1 != p2 {
		return "", errors.New("passphrases do not match")
	}
	return p1, nil
}

func getPassphrase(cmd *cobra.Command, promptMessage string) (string, error) {
	passwordFromArg, err := cmd.Flags().GetString(passwordArgCmdName)
	if err != nil {
		return "", err
	}
	if passwordFromArg != "" {
		return passwordFromArg, nil
	}
	return readPassword(promptMessage)
}

func readPassword(promptMessage string) (string, error) {
	consoleWriter.Print(promptMessage)
	passwordBytes, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return "", err
	}
	consoleWriter.Println("") // line break after reading password
	return string(passwordBytes), nil
}

func pubKeyHexToBytes(s string) ([]byte, bool) {
	if len(s) != 68 {
		return nil, false
	}
	pubKeyBytes, err := hexutil.Decode(s)
	if err != nil {
		return nil, false
	}
	return pubKeyBytes, true
}
