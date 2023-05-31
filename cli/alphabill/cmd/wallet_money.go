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

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/term"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
)

type walletConfig struct {
	Base          *baseConfiguration
	WalletHomeDir string
	LogLevel      string
	LogFile       string
}

const (
	defaultAlphabillNodeURL = "localhost:9543"
	defaultAlphabillApiURL  = "localhost:9654"
	passwordPromptUsage     = "password (interactive from prompt)"
	passwordArgUsage        = "password (non-interactive from args)"

	alphabillNodeURLCmdName = "alphabill-uri"
	alphabillApiURLCmdName  = "alphabill-api-uri"
	seedCmdName             = "seed"
	addressCmdName          = "address"
	amountCmdName           = "amount"
	passwordPromptCmdName   = "password"
	passwordArgCmdName      = "pn"
	logFileCmdName          = "log-file"
	logLevelCmdName         = "log-level"
	walletLocationCmdName   = "wallet-location"
	keyCmdName              = "key"
	waitForConfCmdName      = "wait-for-confirmation"
	totalCmdName            = "total"
	quietCmdName            = "quiet"
	showUnswappedCmdName    = "show-unswapped"
)

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(baseConfig *baseConfiguration) *cobra.Command {
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
	}
	walletCmd.AddCommand(newWalletBillsCmd(config))
	walletCmd.AddCommand(newWalletFeesCmd(context.Background(), config))
	walletCmd.AddCommand(createCmd(config))
	walletCmd.AddCommand(sendCmd(config))
	walletCmd.AddCommand(getPubKeysCmd(config))
	walletCmd.AddCommand(getBalanceCmd(config))
	walletCmd.AddCommand(collectDustCmd(config))
	walletCmd.AddCommand(addKeyCmd(config))
	walletCmd.AddCommand(tokenCmd(config))
	// add passwords flags for (encrypted)wallet
	walletCmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	walletCmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
	walletCmd.PersistentFlags().StringVar(&config.LogFile, logFileCmdName, "", "log file path (default output to stderr)")
	walletCmd.PersistentFlags().StringVar(&config.LogLevel, logLevelCmdName, "INFO", "logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)")
	walletCmd.PersistentFlags().StringVarP(&config.WalletHomeDir, walletLocationCmdName, "l", "", "wallet home directory (default $AB_HOME/wallet)")
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

func execCreateCmd(cmd *cobra.Command, config *walletConfig) (err error) {
	mnemonic := ""
	if cmd.Flags().Changed(seedCmdName) {
		// when user omits value for "s" flag, ie by executing
		// wallet create -s --wallet-location some/path
		// then Cobra eats next param name (--wallet-location) as value for "s". So we validate the mnemonic here to
		// catch this case as otherwise we most likely get error about creating wallet db which is confusing
		if mnemonic, err = cmd.Flags().GetString(seedCmdName); err != nil {
			return fmt.Errorf("failed to read the value of the %q flag: %w", seedCmdName, err)
		}
		if !bip39.IsMnemonicValid(mnemonic) {
			return fmt.Errorf("invalid value %q for flag %q (mnemonic)", mnemonic, seedCmdName)
		}
	}

	password, err := createPassphrase(cmd)
	if err != nil {
		return err
	}

	am, err := account.NewManager(config.WalletHomeDir, password, true)
	if err != nil {
		return fmt.Errorf("failed to create account manager: %w", err)
	}
	defer am.Close()

	if err := money.CreateNewWallet(am, mnemonic); err != nil {
		return fmt.Errorf("failed to create new wallet: %w", err)
	}

	if mnemonic == "" {
		mnemonicSeed, err := am.GetMnemonic()
		if err != nil {
			return fmt.Errorf("failed to read mnemonic created for the wallet: %w", err)
		}
		consoleWriter.Println("The following mnemonic key can be used to recover your wallet. Please write it down now, and keep it in a safe, offline place.")
		consoleWriter.Println("mnemonic key: " + mnemonicSeed)
	}
	return nil
}

func sendCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSendCmd(cmd.Context(), cmd, config)
		},
	}
	cmd.Flags().StringP(addressCmdName, "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	cmd.Flags().StringP(amountCmdName, "v", "", "the amount to send to the receiver")
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill uri to connect to")
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
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
	nodeUri, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return err
	}
	apiUri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.New(apiUri)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{Uri: nodeUri}, am, restClient)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	receiverPubKeyHex, err := cmd.Flags().GetString(addressCmdName)
	if err != nil {
		return err
	}
	receiverPubKey, ok := pubKeyHexToBytes(receiverPubKeyHex)
	if !ok {
		return errors.New("address in not in valid format")
	}
	if len(receiverPubKey) != abcrypto.CompressedSecp256K1PublicKeySize {
		return money.ErrInvalidPubKey
	}

	amountStr, err := cmd.Flags().GetString(amountCmdName)
	if err != nil {
		return err
	}
	amount, err := stringToAmount(amountStr, 8)
	if err != nil {
		return err
	}
	if amount == 0 {
		return fmt.Errorf("invalid parameter \"%s\" for \"--amount\":0 is not valid amount", amountStr)
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return fmt.Errorf("invalid parameter for \"--key\":0 is not a valid account key")
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

	bills, err := w.Send(ctx, money.SendCmd{ReceiverPubKey: receiverPubKey, Amount: amount, WaitForConfirmation: waitForConf, AccountIndex: accountNumber - 1})
	if err != nil {
		return err
	}
	if waitForConf {
		consoleWriter.Println("Successfully confirmed transaction(s)")
		if outputPath != "" {
			var outputBills []*wallet.Bill
			for _, b := range bills {
				outputBills = append(outputBills, b.ToProto())
			}
			outputFile, err := writeBillsToFile(outputPath, outputBills...)
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
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
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
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.New(uri)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{}, am, restClient)
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
		totals, sum, err := w.GetBalances(money.GetBalanceCmd{CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		if !total {
			for i, v := range totals {
				consoleWriter.Println(fmt.Sprintf("#%d %s", i+1, amountToString(v, 8)))
			}
		}
		sumStr := amountToString(sum, 8)
		if quiet {
			consoleWriter.Println(sumStr)
		} else {
			consoleWriter.Println(fmt.Sprintf("Total %s", sumStr))
		}
	} else {
		balance, err := w.GetBalance(money.GetBalanceCmd{AccountIndex: accountNumber - 1, CountDCBills: showUnswapped})
		if err != nil {
			return err
		}
		balanceStr := amountToString(balance, 8)
		if quiet {
			consoleWriter.Println(balanceStr)
		} else {
			consoleWriter.Println(fmt.Sprintf("#%d %s", accountNumber, balanceStr))
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
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	pubKeys, err := am.GetPublicKeys()
	if err != nil {
		return err
	}
	hideKeyNumber, _ := cmd.Flags().GetBool(quietCmdName)
	for accIdx, accPubKey := range pubKeys {
		if hideKeyNumber {
			consoleWriter.Println(hexutil.Encode(accPubKey))
		} else {
			consoleWriter.Println(fmt.Sprintf("#%d %s", accIdx+1, hexutil.Encode(accPubKey)))
		}
	}
	return nil
}

func collectDustCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "consolidates bills",
		Long:  "consolidates all bills into a single bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill uri to connect to")
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "which key to use for dust collection, 0 for all bills from all accounts")
	return cmd
}

func execCollectDust(cmd *cobra.Command, config *walletConfig) error {
	nodeUri, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return err
	}
	apiUri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.New(apiUri)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}

	w, err := money.LoadExistingWallet(client.AlphabillClientConfig{Uri: nodeUri}, am, restClient)
	if err != nil {
		return err
	}
	defer w.Shutdown()

	consoleWriter.Println("Starting dust collection, this may take a while...")
	err = w.CollectDust(context.Background(), accountNumber)

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
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	accIdx, accPubKey, err := am.AddAccount()
	if err != nil {
		return err
	}
	consoleWriter.Println(fmt.Sprintf("Added key #%d %s", accIdx+1, hexutil.Encode(accPubKey)))
	return nil
}

func loadExistingAccountManager(cmd *cobra.Command, walletDir string) (account.Manager, error) {
	pw, err := getPassphrase(cmd, "Enter passphrase: ")
	if err != nil {
		return nil, err
	}
	am, err := account.NewManager(walletDir, pw, false)
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
	passwordFlag, err := cmd.Flags().GetBool(passwordPromptCmdName)
	if err != nil {
		return "", err
	}
	if !passwordFlag {
		return "", nil
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
