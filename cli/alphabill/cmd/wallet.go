package cmd

import (
	"context"
	"errors"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	wlog "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"os"
	"path"
	"strings"
)

const defaultAlphabillUri = "localhost:9543"
const passwordUsage = "password used to encrypt sensitive data"
const logFileCmdName = "log-file"
const logLevelCmdName = "log-level"

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
			return initWalletLogger(cmd, baseConfig.HomeDir)
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Error: must specify a subcommand create, sync, send, get-balance, get-pubkey or collect-dust")
		},
	}
	walletCmd.AddCommand(createCmd(baseConfig))
	walletCmd.AddCommand(syncCmd(baseConfig))
	walletCmd.AddCommand(getBalanceCmd(baseConfig))
	walletCmd.AddCommand(getPubKeyCmd(baseConfig))
	walletCmd.AddCommand(sendCmd(baseConfig))
	walletCmd.AddCommand(collectDustCmd(baseConfig))
	walletCmd.PersistentFlags().String(logFileCmdName, "", fmt.Sprintf("log file path (default $AB_HOME/wallet/wallet.log)"))
	walletCmd.PersistentFlags().String(logLevelCmdName, "INFO", fmt.Sprintf("logging level [%s]", walletLogLevels()))
	return walletCmd
}

func createCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCreateCmd(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("seed", "s", "", "mnemonic seed, the number of words should be 12, 15, 18, 21 or 24")
	cmd.Flags().StringP("password", "p", "", passwordUsage)
	return cmd
}

func execCreateCmd(cmd *cobra.Command, walletDir string) error {
	mnemonic, err := cmd.Flags().GetString("seed")
	if err != nil {
		return err
	}
	password, err := cmd.Flags().GetString("password")
	if err != nil {
		return err
	}
	var w *wallet.Wallet
	if mnemonic != "" {
		fmt.Println("Creating wallet from mnemonic seed...")
		w, err = wallet.CreateWalletFromSeed(mnemonic, wallet.Config{DbPath: walletDir, WalletPass: password})
	} else {
		fmt.Println("Creating new wallet...")
		w, err = wallet.CreateNewWallet(wallet.Config{DbPath: walletDir, WalletPass: password})
	}
	if err != nil {
		return err
	}
	defer w.Shutdown()
	fmt.Println("Wallet successfully created")
	return nil
}

func syncCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSyncCmd(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().StringP("password", "p", "", passwordUsage)
	return cmd
}

func execSyncCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
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
			return execSendCmd(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("address", "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	cmd.Flags().Uint64P("amount", "v", 0, "the amount to send to the receiver")
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().StringP("password", "p", "", passwordUsage)
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

func execSendCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
	if err != nil {
		return err
	}
	w, err := loadExistingWallet(cmd, walletDir, uri)
	if err != nil {
		return err
	}
	pubKeyHex, err := cmd.Flags().GetString("address")
	if err != nil {
		return err
	}
	pubKey, ok := pubKeyHexToBytes(pubKeyHex)
	if !ok {
		return errors.New("address in not in valid format")
	}
	amount, err := cmd.Flags().GetUint64("amount")
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
			return execGetBalanceCmd(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("password", "p", "", passwordUsage)
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
			return execGetPubKeyCmd(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("password", "p", "", passwordUsage)
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
		Short: "collect-dust consolidates bills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, baseConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().StringP("password", "p", "", passwordUsage)
	return cmd
}

func execCollectDust(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
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
	walletPass, err := cmd.Flags().GetString("password")
	if err != nil {
		return nil, err
	}
	return wallet.LoadExistingWallet(wallet.Config{
		DbPath:                walletDir,
		WalletPass:            walletPass,
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: uri},
	})
}

func walletLogLevels() string {
	keys := make([]string, 0, len(wlog.Levels))
	for k := range wlog.Levels {
		keys = append(keys, k)
	}
	return strings.Join(keys, "/")
}

func initWalletLogger(cmd *cobra.Command, homeDir string) error {
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
		logFilePath = path.Join(homeDir, "wallet")
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
