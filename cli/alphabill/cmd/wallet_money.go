package cmd

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"syscall"

	moneytx "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
	"github.com/tyler-smith/go-bip39"
	"golang.org/x/term"

	"github.com/alphabill-org/alphabill/client/wallet/account"
	"github.com/alphabill-org/alphabill/client/wallet/money"
	moneyclient "github.com/alphabill-org/alphabill/client/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/client/wallet/unitlock"
)

type walletConfig struct {
	Base          *baseConfiguration
	WalletHomeDir string
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
			if err := initializeConfig(cmd, baseConfig); err != nil {
				return fmt.Errorf("initializing base configuration: %w", err)
			}
			if err := initWalletConfig(cmd, config); err != nil {
				return fmt.Errorf("initializing wallet configuration: %w", err)
			}
			return nil
		},
	}
	walletCmd.AddCommand(newWalletBillsCmd(config))
	walletCmd.AddCommand(newWalletFeesCmd(config))
	walletCmd.AddCommand(createCmd(config))
	walletCmd.AddCommand(sendCmd(config))
	walletCmd.AddCommand(getPubKeysCmd(config))
	walletCmd.AddCommand(getBalanceCmd(config))
	walletCmd.AddCommand(collectDustCmd(config))
	walletCmd.AddCommand(addKeyCmd(config))
	walletCmd.AddCommand(tokenCmd(config))
	walletCmd.AddCommand(evmCmd(config))
	// add passwords flags for (encrypted)wallet
	walletCmd.PersistentFlags().BoolP(passwordPromptCmdName, "p", false, passwordPromptUsage)
	walletCmd.PersistentFlags().String(passwordArgCmdName, "", passwordArgUsage)
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
	cmd.Flags().StringSliceP(addressCmdName, "a", nil, "compressed secp256k1 public key(s) of "+
		"the receiver(s) in hexadecimal format, must start with 0x and be 68 characters in length, must match with "+
		"amounts")
	cmd.Flags().StringSliceP(amountCmdName, "v", nil, "the amount(s) to send to the "+
		"receiver(s), must match with addresses")
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	// use string instead of boolean as boolean requires equals sign between name and value e.g. w=[true|false]
	cmd.Flags().StringP(waitForConfCmdName, "w", "true", "waits for transaction confirmation on the blockchain, otherwise just broadcasts the transaction")
	if err := cmd.MarkFlagRequired(addressCmdName); err != nil {
		panic(err)
	}
	if err := cmd.MarkFlagRequired(amountCmdName); err != nil {
		panic(err)
	}
	return cmd
}

func execSendCmd(ctx context.Context, cmd *cobra.Command, config *walletConfig) error {
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
	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	w, err := money.LoadExistingWallet(am, unitLocker, restClient, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	if accountNumber == 0 {
		return fmt.Errorf("invalid parameter for flag %q: 0 is not a valid account key", keyCmdName)
	}
	waitForConfStr, err := cmd.Flags().GetString(waitForConfCmdName)
	if err != nil {
		return err
	}
	waitForConf, err := strconv.ParseBool(waitForConfStr)
	if err != nil {
		return err
	}
	receiverPubKeys, err := cmd.Flags().GetStringSlice(addressCmdName)
	if err != nil {
		return err
	}
	receiverAmounts, err := cmd.Flags().GetStringSlice(amountCmdName)
	if err != nil {
		return err
	}
	receivers, err := groupPubKeysAndAmounts(receiverPubKeys, receiverAmounts)
	if err != nil {
		return err
	}
	proofs, err := w.Send(ctx, money.SendCmd{Receivers: receivers, WaitForConfirmation: waitForConf, AccountIndex: accountNumber - 1})
	if err != nil {
		return err
	}
	if waitForConf {
		consoleWriter.Println("Successfully confirmed transaction(s)")

		var feeSum uint64
		for _, proof := range proofs {
			feeSum += proof.TxRecord.ServerMetadata.GetActualFee()
		}
		consoleWriter.Println("Paid", amountToString(feeSum, 8), "fees for transaction(s).")
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
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	w, err := money.LoadExistingWallet(am, unitLocker, restClient, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

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
		totals, sum, err := w.GetBalances(cmd.Context(), money.GetBalanceCmd{CountDCBills: showUnswapped})
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
		balance, err := w.GetBalance(cmd.Context(), money.GetBalanceCmd{AccountIndex: accountNumber - 1, CountDCBills: showUnswapped})
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
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "which key to use for dust collection, 0 for all bills from all accounts")
	return cmd
}

func execCollectDust(cmd *cobra.Command, config *walletConfig) error {
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
	defer am.Close()

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer unitLocker.Close()

	w, err := money.LoadExistingWallet(am, unitLocker, restClient, config.Base.Logger)
	if err != nil {
		return err
	}
	defer w.Close()

	consoleWriter.Println("Starting dust collection, this may take a while...")
	dcResults, err := w.CollectDust(cmd.Context(), accountNumber)
	if err != nil {
		consoleWriter.Println("Failed to collect dust: " + err.Error())
		return err
	}
	for _, dcResult := range dcResults {
		if dcResult.SwapProof != nil {
			attr := &moneytx.SwapDCAttributes{}
			err := dcResult.SwapProof.TxRecord.TransactionOrder.UnmarshalAttributes(attr)
			if err != nil {
				return fmt.Errorf("failed to unmarshal swap tx proof: %w", err)
			}
			consoleWriter.Println(fmt.Sprintf(
				"Dust collection finished successfully on account #%d. Joined %d bills with total value of %s "+
					"ALPHA into an existing target bill with unit identifier 0x%s. Paid %s fees for transaction(s).",
				dcResult.AccountIndex+1,
				len(attr.DcTransfers),
				amountToString(attr.TargetValue, 8),
				dcResult.SwapProof.TxRecord.TransactionOrder.UnitID(),
				amountToString(dcResult.FeeSum, 8),
			))
		} else {
			consoleWriter.Println(fmt.Sprintf("Nothing to swap on account #%d", dcResult.AccountIndex+1))
		}
	}
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

func groupPubKeysAndAmounts(pubKeys []string, amounts []string) ([]money.ReceiverData, error) {
	if len(pubKeys) != len(amounts) {
		return nil, fmt.Errorf("must specify the same amount of addresses and amounts")
	}
	var receivers []money.ReceiverData
	for i := 0; i < len(pubKeys); i++ {
		amount, err := stringToAmount(amounts[i], 8)
		if err != nil {
			return nil, fmt.Errorf("invalid amount: %w", err)
		}
		pubKeyBytes, err := hexutil.Decode(pubKeys[i])
		if err != nil {
			return nil, fmt.Errorf("invalid address format: %s", pubKeys[i])
		}
		receivers = append(receivers, money.ReceiverData{
			Amount: amount,
			PubKey: pubKeyBytes,
		})
	}
	return receivers, nil
}
