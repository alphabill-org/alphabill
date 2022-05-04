package cmd

import (
	"context"
	"errors"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"syscall"
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
)

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	// TODO wallet-sdk log statements should probably not appear to console i.e.
	// ./alphabill wallet get-balance
	// 150
	// [I]{0001}2022/02/11 11:20:21.128808 wallet.go:192: Shutting down wallet
	// [I]{0001}2022/02/11 11:20:21.128882 walletdb.go:376: Closing wallet db
	//
	// we can set SDKs logger by log.SetLogger(logger.CreateForPackage())
	// however, the given and expected loggers seem to be incompatible
	var walletCmd = &cobra.Command{
		Use:   "wallet",
		Short: "cli for managing alphabill wallet",
		Long:  "cli for managing alphabill wallet",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// overrides parent PersistentPreRunE so that logger will not get initialized for wallet subcommand
			return nil
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
	return walletCmd
}

func createCmd(baseConfig *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCreateCmd(cmd, baseConfig.HomeDir)
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
			return execSendCmd(cmd, baseConfig.HomeDir)
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
			return execGetBalanceCmd(cmd, baseConfig.HomeDir)
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
			return execGetPubKeyCmd(cmd, baseConfig.HomeDir)
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
		Short: "collect-dust consolidates bills",
		Long:  "collect-dust consolidates bills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, baseConfig.HomeDir)
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
	walletPass, err := getPassphrase(cmd, "Enter passphrase: ")
	if err != nil {
		return nil, err
	}
	return wallet.LoadExistingWallet(wallet.Config{
		DbPath:                walletDir,
		WalletPass:            walletPass,
		AlphabillClientConfig: wallet.AlphabillClientConfig{Uri: uri},
	})
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
	fmt.Println() // insert empty line between two prompts
	p2, err := readPassword("Confirm passphrase: ")
	if err != nil {
		return "", err
	}
	fmt.Println() // insert empty line after reading prompts
	if p1 != p2 {
		return "", errors.New("passphrases do not much")
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
	fmt.Print(promptMessage)
	passwordBytes, err := term.ReadPassword(syscall.Stdin)
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}

func addPasswordFlags(cmd *cobra.Command) {
	cmd.Flags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	cmd.Flags().String(passwordArgCmdName, "", passwordArgUsage)
}
