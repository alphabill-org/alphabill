package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/client/wallet"
	"github.com/alphabill-org/alphabill/client/wallet/money/backend"
	"github.com/alphabill-org/alphabill/client/wallet/money/backend/client"
	"github.com/alphabill-org/alphabill/client/wallet/unitlock"
	"github.com/alphabill-org/alphabill/validator/pkg/network/protocol/genesis"
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

	unitLocker, err := unitlock.NewUnitLocker(config.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("failed to open unit locker: %w", err)
	}
	defer unitLocker.Close()

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
			billValueStr := amountToString(bill.Value, 8)
			lockedReasonStr, err := getLockedReasonString(group.pubKey, unitLocker, bill)
			if err != nil {
				return err
			}
			consoleWriter.Println(fmt.Sprintf("#%d 0x%X %s%s", j+1, bill.Id, billValueStr, lockedReasonStr))
		}
	}
	return nil
}

func getLockedReasonString(accountID []byte, unitLocker *unitlock.UnitLocker, bill *wallet.Bill) (string, error) {
	lockedUnit, err := unitLocker.GetUnit(accountID, bill.GetID())
	if err != nil {
		return "", fmt.Errorf("failed to load locked unit: %w", err)
	}
	if lockedUnit != nil {
		return fmt.Sprintf(" (%s)", lockedUnit.LockReason.String()), nil
	}
	return "", nil
}
