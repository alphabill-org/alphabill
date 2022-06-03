package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"syscall"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	wlog "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/money"
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
)

type walletConfig struct {
	Base          *baseConfiguration
	WalletHomeDir string
	LogLevel      string
	LogFile       string
}

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(_ context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &walletConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "cli for managing alphabill wallet",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// initalize config so that baseConfig.HomeDir gets configured
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
			fmt.Println("Error: must specify a subcommand like create, sync, send etc")
		},
	}
	walletCmd.AddCommand(createCmd(config))
	walletCmd.AddCommand(syncCmd(config))
	walletCmd.AddCommand(getBalanceCmd(config))
	walletCmd.AddCommand(getPubKeyCmd(config))
	walletCmd.AddCommand(sendCmd(config))
	walletCmd.AddCommand(collectDustCmd(config))
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
	addPasswordFlags(cmd)
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
	c := money.WalletConfig{DbPath: config.WalletHomeDir, WalletPass: password}
	var w *money.Wallet
	fmt.Println("Creating new wallet...")
	w, err = money.CreateNewWallet(mnemonic, c)
	if err != nil {
		return err
	}
	defer w.Shutdown()
	fmt.Println("Wallet created successfully.")

	// print mnemonic if new wallet was created
	if mnemonic == "" {
		mnemonicSeed, err := w.GetMnemonic()
		if err != nil {
			return err
		}
		fmt.Println("The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
		fmt.Println("mnemonic key: " + mnemonicSeed)
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
	addPasswordFlags(cmd)
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

	err = w.SyncToMaxBlockNumber(ctx)
	if err != nil {
		fmt.Println("Failed to synchronize wallet: " + err.Error())
		return err
	}
	fmt.Println("Wallet synchronized successfully.")
	return nil
}

func sendCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSendCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	cmd.Flags().Uint64P(amountCmdName, "v", 0, "the amount to send to the receiver")
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	addPasswordFlags(cmd)
	_ = cmd.MarkFlagRequired(addressCmdName)
	_ = cmd.MarkFlagRequired(amountCmdName)
	return cmd
}

func execSendCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, uri)
	if err != nil {
		return err
	}
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
	err = w.Send(pubKey, amount)
	if err != nil {
		return err
	}
	fmt.Println("successfully sent transaction")
	return nil
}

func getBalanceCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetBalanceCmd(cmd, config)
		},
	}
	addPasswordFlags(cmd)
	return cmd
}

func execGetBalanceCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	balance, err := w.GetBalance()
	if err != nil {
		return err
	}
	fmt.Println(balance)
	return nil
}

func getPubKeyCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-pubkey",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetPubKeyCmd(cmd, config)
		},
	}
	addPasswordFlags(cmd)
	return cmd
}

func execGetPubKeyCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	pubKey, err := w.GetPublicKey()
	if err != nil {
		return err
	}
	fmt.Println(hexutil.Encode(pubKey))
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
	addPasswordFlags(cmd)
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
	if err != nil {
		return err
	}
	defer w.Shutdown()

	fmt.Println("Starting dust collection, this may take a while...")
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
		return err
	}
	fmt.Println("Dust collection finished successfully.")
	return nil
}

func loadExistingWallet(cmd *cobra.Command, walletDir string, uri string) (*money.Wallet, error) {
	config := money.WalletConfig{
		DbPath:                walletDir,
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: uri},
	}
	isEncrypted, err := money.IsEncrypted(config)
	if err != nil {
		return nil, err
	}
	if isEncrypted {
		walletPass, err := getPassphrase(cmd, "Enter passphrase: ")
		if err != nil {
			return nil, err
		}
		config.WalletPass = walletPass
	}
	return money.LoadExistingWallet(config)
}

func initWalletConfig(cmd *cobra.Command, config *walletConfig) error {
	walletLocation, err := cmd.Flags().GetString(walletLocationCmdName)
	if err != nil {
		return err
	}
	if walletLocation != "" {
		config.WalletHomeDir = walletLocation
	} else {
		config.WalletHomeDir = path.Join(config.Base.HomeDir, "wallet")
	}
	return nil
}

func initWalletLogger(config *walletConfig) error {
	var logWriter io.Writer
	if config.LogFile != "" {
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
	fmt.Print(promptMessage)
	passwordBytes, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return "", err
	}
	fmt.Println() // line break after reading password
	return string(passwordBytes), nil
}

func addPasswordFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.Flags().String(passwordArgCmdName, "", passwordArgUsage)
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
