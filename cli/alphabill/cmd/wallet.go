package cmd

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"github.com/spf13/cobra"
	"strings"
)

const defaultAlphabillUri = "localhost:9543"

// newWalletCmd creates a new cobra command for the wallet component.
func newWalletCmd(ctx context.Context, rootConfig *rootConfiguration) *cobra.Command {
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
			fmt.Println("Error: must specify a subcommand create, sync, send, get-balance or collect-dust")
		},
	}
	walletCmd.AddCommand(createCmd(rootConfig))
	walletCmd.AddCommand(syncCmd(rootConfig))
	walletCmd.AddCommand(getBalanceCmd(rootConfig))
	walletCmd.AddCommand(sendCmd(rootConfig))
	walletCmd.AddCommand(collectDustCmd(rootConfig))
	return walletCmd
}

func createCmd(rootConfig *rootConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			mnemonic, err := cmd.Flags().GetString("seed")
			if err != nil {
				return err
			}
			return execCreateCmd(rootConfig.HomeDir, mnemonic)
		},
	}
	cmd.Flags().StringP("seed", "s", "", "mnemonic seed, the number of words should be 12, 15, 18, 21 or 24")
	return cmd
}

func execCreateCmd(walletDir string, mnemonic string) error {
	var w *wallet.Wallet
	var err error
	if mnemonic != "" {
		fmt.Println("Creating wallet from mnemonic seed...")
		w, err = wallet.CreateWalletFromSeed(mnemonic, wallet.Config{DbPath: walletDir})
	} else {
		fmt.Println("Creating new wallet...")
		w, err = wallet.CreateNewWallet(wallet.Config{DbPath: walletDir})
	}
	if err != nil {
		return err
	}
	defer w.Shutdown()
	fmt.Println("Wallet successfully created")
	return nil
}

func syncCmd(rootConfig *rootConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "sync",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSyncCmd(cmd, rootConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	return cmd
}

func execSyncCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
	if err != nil {
		return err
	}
	w, err := wallet.LoadExistingWallet(wallet.Config{DbPath: walletDir, AlphaBillClientConfig: wallet.AlphaBillClientConfig{Uri: uri}})
	if err != nil {
		return err
	}
	defer w.Shutdown()
	return w.SyncToMaxBlockHeight()
}

func sendCmd(rootConfig *rootConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "send",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execSendCmd(cmd, rootConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("address", "a", "", "compressed secp256k1 public key of the receiver in hexadecimal format, must start with 0x and be 68 characters in length")
	cmd.Flags().Uint64P("amount", "v", 0, "the amount to send to the receiver")
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("amount")
	return cmd
}

func execSendCmd(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
	if err != nil {
		return err
	}
	w, err := wallet.LoadExistingWallet(wallet.Config{DbPath: walletDir, AlphaBillClientConfig: wallet.AlphaBillClientConfig{Uri: uri}})
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

func getBalanceCmd(rootConfig *rootConfiguration) *cobra.Command {
	return &cobra.Command{
		Use: "get-balance",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execGetBalanceCmd(rootConfig.HomeDir)
		},
	}
}

func execGetBalanceCmd(walletDir string) error {
	w, err := wallet.LoadExistingWallet(wallet.Config{DbPath: walletDir})
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

func collectDustCmd(rootConfig *rootConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collect-dust",
		Short: "collect-dust consolidates bills",
		Long:  "collect-dust consolidates bills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execCollectDust(cmd, rootConfig.HomeDir)
		},
	}
	cmd.Flags().StringP("alphabill-uri", "u", defaultAlphabillUri, "alphabill uri to connect to")
	return cmd
}

func execCollectDust(cmd *cobra.Command, walletDir string) error {
	uri, err := cmd.Flags().GetString("alphabill-uri")
	if err != nil {
		return err
	}
	w, err := wallet.LoadExistingWallet(wallet.Config{DbPath: walletDir, AlphaBillClientConfig: wallet.AlphaBillClientConfig{Uri: uri}})
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
	if !strings.HasPrefix(s, "0x") {
		return nil, false
	}
	pubKeyBytes, err := hex.DecodeString(s[2:])
	if err != nil {
		return nil, false
	}
	return pubKeyBytes, true
}