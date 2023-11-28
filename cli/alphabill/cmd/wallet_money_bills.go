package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend"
	"github.com/alphabill-org/alphabill/pkg/wallet/money/backend/client"
	txbuilder "github.com/alphabill-org/alphabill/pkg/wallet/money/tx_builder"
)

type (
	// TrustBase json schema for trust base file.
	TrustBase struct {
		RootValidators []*genesis.PublicKeyInfo `json:"root_validators"`
	}
)

// newWalletBillsCmd creates a new cobra command for the wallet bills component.
func newWalletBillsCmd(config *walletConfig) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "bills",
		Short: "cli for managing alphabill wallet bills and proofs",
	}
	cmd.AddCommand(listCmd(config))
	cmd.AddCommand(lockCmd(config))
	cmd.AddCommand(unlockCmd(config))
	return cmd
}

func listCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "lists bill ids and values",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execListCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account bills to list (default: all accounts)")
	cmd.Flags().BoolP(showUnswappedCmdName, "s", false, "includes unswapped dust bills in output")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	showUnswapped, err := cmd.Flags().GetBool(showUnswappedCmdName)
	if err != nil {
		return err
	}

	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()

	type accountBillGroup struct {
		accountIndex uint64
		pubKey       []byte
		bills        *backend.ListBillsResponse
	}
	var accountBillGroups []*accountBillGroup
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		for accountIndex, pubKey := range pubKeys {
			bills, err := restClient.ListBills(cmd.Context(), pubKey, showUnswapped, "", 100)
			if err != nil {
				return err
			}
			accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: pubKey, accountIndex: uint64(accountIndex), bills: bills})
		}
	} else {
		accountIndex := accountNumber - 1
		pubKey, err := am.GetPublicKey(accountIndex)
		if err != nil {
			return err
		}
		accountBills, err := restClient.ListBills(cmd.Context(), pubKey, showUnswapped, "", 100)
		if err != nil {
			return err
		}
		accountBillGroups = append(accountBillGroups, &accountBillGroup{pubKey: pubKey, accountIndex: accountIndex, bills: accountBills})
	}

	for _, group := range accountBillGroups {
		if len(group.bills.Bills) == 0 {
			consoleWriter.Println(fmt.Sprintf("Account #%d - empty", group.accountIndex+1))
		} else {
			consoleWriter.Println(fmt.Sprintf("Account #%d", group.accountIndex+1))
		}
		for j, bill := range group.bills.Bills {
			billValueStr := util.AmountToString(bill.Value, 8)
			consoleWriter.Println(fmt.Sprintf("#%d 0x%X %s%s", j+1, bill.Id, billValueStr, getLockedReasonString(bill)))
		}
	}
	return nil
}

func lockCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock",
		Short: "locks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execLockCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "account number of the bill to lock")
	cmd.Flags().BytesHex(billIdCmdName, nil, "id of the bill to lock")
	cmd.Flags().BytesHex(systemIdentifierCmdName, moneytx.DefaultSystemIdentifier, "system identifier")
	return cmd
}

func execLockCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	billID, err := cmd.Flags().GetBytesHex(billIdCmdName)
	if err != nil {
		return err
	}
	systemID, err := cmd.Flags().GetBytesHex(systemIdentifierCmdName)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()
	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}
	bill, err := fetchBillByID(cmd.Context(), billID, restClient, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch bill by id: %w", err)
	}
	if bill.IsLocked() {
		return errors.New("bill is already locked")
	}
	roundNumber, err := restClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := txbuilder.NewLockTx(accountKey, systemID, bill.Id, bill.TxHash, wallet.LockReasonManual, roundNumber+10)
	if err != nil {
		return fmt.Errorf("failed to create lock tx: %w", err)
	}
	moneyTxPublisher := money.NewTxPublisher(restClient, config.Base.Logger)
	_, err = moneyTxPublisher.SendTx(cmd.Context(), tx, accountKey.PubKey)
	if err != nil {
		return fmt.Errorf("failed to send lock tx: %w", err)
	}
	consoleWriter.Println("Bill locked successfully.")
	return nil
}

func unlockCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock",
		Short: "unlocks specific bill",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execUnlockCmd(cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "r", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "account number of the bill to unlock")
	cmd.Flags().BytesHex(billIdCmdName, nil, "id of the bill to unlock")
	cmd.Flags().BytesHex(systemIdentifierCmdName, moneytx.DefaultSystemIdentifier, "system identifier")
	return cmd
}

func execUnlockCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := client.New(uri)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	billID, err := cmd.Flags().GetBytesHex(billIdCmdName)
	if err != nil {
		return err
	}
	systemID, err := cmd.Flags().GetBytesHex(systemIdentifierCmdName)
	if err != nil {
		return err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to load account manager: %w", err)
	}
	defer am.Close()

	accountKey, err := am.GetAccountKey(accountNumber - 1)
	if err != nil {
		return fmt.Errorf("failed to load account key: %w", err)
	}
	bill, err := fetchBillByID(cmd.Context(), billID, restClient, accountKey)
	if err != nil {
		return fmt.Errorf("failed to fetch bill by id: %w", err)
	}
	if !bill.IsLocked() {
		return errors.New("bill is already unlocked")
	}
	roundNumber, err := restClient.GetRoundNumber(cmd.Context())
	if err != nil {
		return fmt.Errorf("failed to fetch round number: %w", err)
	}
	tx, err := txbuilder.NewUnlockTx(accountKey, systemID, bill, roundNumber+10)
	if err != nil {
		return fmt.Errorf("failed to create unlock tx: %w", err)
	}
	moneyTxPublisher := money.NewTxPublisher(restClient, config.Base.Logger)
	_, err = moneyTxPublisher.SendTx(cmd.Context(), tx, accountKey.PubKey)
	if err != nil {
		return fmt.Errorf("failed to send unlock tx: %w", err)
	}
	consoleWriter.Println("Bill unlocked successfully.")
	return nil
}

func fetchBillByID(ctx context.Context, billID []byte, restClient *client.MoneyBackendClient, accountKey *account.AccountKey) (*wallet.Bill, error) {
	bills, err := restClient.GetBills(ctx, accountKey.PubKey)
	if err != nil {
		return nil, err
	}
	for _, b := range bills {
		if bytes.Equal(b.Id, billID) {
			return b, nil
		}
	}
	return nil, errors.New("bill not found")
}

func getLockedReasonString(bill *wallet.Bill) string {
	if bill.IsLocked() {
		return fmt.Sprintf(" (%s)", bill.Locked.String())
	}
	return ""
}
