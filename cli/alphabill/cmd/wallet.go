package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"syscall"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	wlog "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
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
)

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(_ context.Context, baseConfig *baseConfiguration) *cobra.Command {
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "cli for managing alphabill wallet",
		Long:  "cli for managing alphabill wallet",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// initalize config so that baseConfig.HomeDir gets configured
			err := initializeConfig(cmd, baseConfig)
			if err != nil {
				return err
			}
			return initWalletLogger(cmd, walletHomeDir(baseConfig))
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Error: must specify a subcommand like create, sync, send etc")
		},
	}
	walletCmd.AddCommand(createCmd(baseConfig))
	walletCmd.AddCommand(syncCmd(baseConfig))
	walletCmd.AddCommand(getBalanceCmd(baseConfig))
	walletCmd.AddCommand(getPubKeyCmd(baseConfig))
	walletCmd.AddCommand(sendCmd(baseConfig))
	walletCmd.AddCommand(collectDustCmd(baseConfig))
	walletCmd.PersistentFlags().String(logFileCmdName, "", fmt.Sprintf("log file path (default $AB_HOME/wallet/wallet.log)"))
	walletCmd.PersistentFlags().String(logLevelCmdName, "INFO", fmt.Sprintf("logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)"))
	return walletCmd
}

func createCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCreateCmd(cmd, walletHomeDir(baseConfig))
		},
	}
	cmd.Flags().StringP(seedCmdName, "s", "", "mnemonic seed, the number of words should be 12, 15, 18, 21 or 24")
	addPasswordFlags(cmd)
	return cmd
}

func execCreateCmd(cmd *cobra.Command, walletDir string) error {
	mnemonic, err := cmd.Flags().GetString(seedCmdName)
	if err != nil {
		return err
	}
	password, err := createPassphrase(cmd)
	if err != nil {
		return err
	}
	config := wallet.Config{DbPath: walletDir, WalletPass: password}
	var w *wallet.Wallet
	if mnemonic != "" {
		fmt.Println("Creating wallet from mnemonic seed...")
		w, err = wallet.CreateWalletFromSeed(mnemonic, config)
	} else {
		fmt.Println("Creating new wallet...")
		w, err = wallet.CreateNewWallet(config)
	}
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

func syncCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSyncCmd(cmd, walletHomeDir(baseConfig))
		},
	}
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	addPasswordFlags(cmd)
	return cmd
}

func execSyncCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, walletDir, uri)
	if err != nil {
		return err
	}
	defer w.Shutdown()
	w.SyncToMaxBlockHeight()
	return nil
}

func sendCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSendCmd(cmd, walletHomeDir(baseConfig))
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

func execSendCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, walletDir, uri)
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
		// TODO convert known errors to normal output messages?
		// i.e. in case of errBillWithMinValueNotFound let user know he should collect dust?
		return err
	}
	fmt.Println("successfully sent transaction")
	return nil
}

func getBalanceCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetBalanceCmd(cmd, walletHomeDir(baseConfig))
		},
	}
	addPasswordFlags(cmd)
	return cmd
}

func execGetBalanceCmd(cmd *cobra.Command, walletDir string) error {
	w, err := loadExistingWallet(cmd, walletDir, "")
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

func getPubKeyCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "get-pubkey",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetPubKeyCmd(cmd, walletHomeDir(baseConfig))
		},
	}
	addPasswordFlags(cmd)
	return cmd
}

func execGetPubKeyCmd(cmd *cobra.Command, walletDir string) error {
	w, err := loadExistingWallet(cmd, walletDir, "")
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

func collectDustCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "consolidates bills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, walletHomeDir(baseConfig))
		},
	}
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	addPasswordFlags(cmd)
	return cmd
}

func execCollectDust(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, walletDir, uri)
	if err != nil {
		return err
	}
	if err != nil {
		return err
	}
	defer w.Shutdown()

	fmt.Println("starting dust collection, this may take a while...")
	err = w.CollectDust()
	if err != nil {
		return err
	}
	fmt.Println("dust collection finished")
	return nil
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

func loadExistingWallet(cmd *cobra.Command, walletDir string, uri string) (*wallet.Wallet, error) {
	config := wallet.Config{
		DbPath:                walletDir,
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: uri},
	}
	isEncrypted, err := wallet.IsEncrypted(config)
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
	return wallet.LoadExistingWallet(config)
}

func walletHomeDir(baseConfig *baseConfiguration) string {
	return path.Join(baseConfig.HomeDir, "wallet")
}

func initWalletLogger(cmd *cobra.Command, walletHomeDir string) error {
	logLevelStr, err := cmd.Flags().GetString(logLevelCmdName)
	if err != nil {
		return err
	}
	logLevel := wlog.Levels[logLevelStr]

	logFilePath, err := cmd.Flags().GetString(logFileCmdName)
	if err != nil {
		return err
	}
	if logFilePath == "" {
		logFilePath = walletHomeDir
		err = os.MkdirAll(logFilePath, 0700) // -rwx------
		if err != nil {
			return err
		}
		logFilePath = path.Join(logFilePath, "wallet.log")
	}

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // -rw-------
	if err != nil {
		return err
	}

	walletLogger, err := wlog.New(logLevel, logFile)
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
