package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	backendmoney "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	moneyclient "github.com/alphabill-org/alphabill/pkg/wallet/backend/money/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const (
	billIdCmdName        = "bill-id"
	outputPathCmdName    = "output-path"
	trustBaseFileCmdName = "trust-base-file"
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
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand")
		},
	}
	cmd.AddCommand(listCmd(config))
	cmd.AddCommand(exportCmd(config))
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
	cmd.Flags().StringP(alphabillApiURLCmdName, "u", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account bills to list (default: all accounts)")
	cmd.Flags().BoolP(showUnswappedCmdName, "s", false, "includes unswapped dust bills in output")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.NewClient(uri)
	if err != nil {
		return err
	}
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
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
		bills        *backendmoney.ListBillsResponse
	}
	var accountBillGroups []*accountBillGroup
	if accountNumber == 0 {
		pubKeys, err := am.GetPublicKeys()
		if err != nil {
			return err
		}
		for accountIndex, pubKey := range pubKeys {
			bills, err := restClient.ListBills(pubKey)
			if err != nil {
				return err
			}
			accountBillGroups = append(accountBillGroups, &accountBillGroup{accountIndex: uint64(accountIndex), bills: bills})
		}
	} else {
		accountIndex := accountNumber - 1
		pubKey, err := am.GetPublicKey(accountIndex)
		if err != nil {
			return err
		}
		accountBills, err := restClient.ListBills(pubKey)
		if err != nil {
			return err
		}
		accountBillGroups = append(accountBillGroups, &accountBillGroup{accountIndex: accountIndex, bills: accountBills})
	}

	for _, group := range accountBillGroups {
		if len(group.bills.Bills) == 0 {
			consoleWriter.Println(fmt.Sprintf("Account #%d - empty", group.accountIndex+1))
		} else {
			consoleWriter.Println(fmt.Sprintf("Account #%d", group.accountIndex+1))
		}
		for j, bill := range group.bills.Bills {
			billValueStr := amountToString(bill.Value, 8)
			consoleWriter.Println(fmt.Sprintf("#%d 0x%X %s", j+1, bill.Id, billValueStr))
		}
	}
	return nil
}

func exportCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export",
		Short: "exports bills to json file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execExportCmd(cmd, config)
		},
		Hidden: true,
	}
	cmd.Flags().StringP(alphabillApiURLCmdName, "u", defaultAlphabillApiURL, "alphabill API uri to connect to")
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies which account bills to export")
	cmd.Flags().BytesHexP(billIdCmdName, "b", nil, "bill ID in hex format (without 0x prefix)")
	cmd.Flags().StringP(outputPathCmdName, "o", "", "output directory for bills, directory is created if it does not exist (default: CWD)")
	return cmd
}

func execExportCmd(cmd *cobra.Command, config *walletConfig) error {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return err
	}
	restClient, err := moneyclient.NewClient(uri)
	if err != nil {
		return err
	}

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	billId, err := cmd.Flags().GetBytesHex(billIdCmdName)
	if err != nil {
		return err
	}
	outputPath, err := cmd.Flags().GetString(outputPathCmdName)
	if err != nil {
		return err
	}
	if outputPath == "" {
		outputPath, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return err
	}
	defer am.Close()
	pk, err := am.GetPublicKey(accountNumber - 1)
	if err != nil {
		return err
	}

	// create directories if output path dir does not exist
	err = os.MkdirAll(outputPath, 0700) // -rwx------
	if err != nil {
		return err
	}
	// export bill using --bill-id if present
	if len(billId) > 0 {
		proof, err := restClient.GetProof(billId)
		if err != nil {
			return err
		}
		outputFile, err := writeBillsToFile(outputPath, proof.Bills...)
		if err != nil {
			return err
		}
		consoleWriter.Println("Exported bill(s) to: " + outputFile)
		return nil
	}
	// export all bills if neither --bill-id or --bill-order-number are given
	billsList, err := restClient.ListBills(pk)
	if err != nil {
		return err
	}

	var bills []*block.Bill
	for _, b := range billsList.Bills {
		proof, err := restClient.GetProof(b.Id)
		if err != nil {
			return err
		}
		bills = append(bills, proof.Bills[0])
	}

	outputFile, err := writeBillsToFile(outputPath, bills...)
	if err != nil {
		return err
	}
	consoleWriter.Println("Exported bill(s) to: " + outputFile)
	return nil
}

// writeBillsToFile writes bill(s) to given directory.
// Creates outputDir if it does not already exist. Returns output file.
func writeBillsToFile(outputDir string, bills ...*block.Bill) (string, error) {
	outputFile, err := getOutputFile(outputDir, bills)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(outputDir, 0700) // -rwx------
	if err != nil {
		return "", err
	}
	err = block.WriteBillsFile(outputFile, &block.Bills{Bills: bills})
	if err != nil {
		return "", err
	}
	return outputFile, nil
}

// getOutputFile returns filename either bill-<bill-id-hex>.json or bills.json
func getOutputFile(outputDir string, bills []*block.Bill) (string, error) {
	if len(bills) == 0 {
		return "", errors.New("no bills to export")
	} else if len(bills) == 1 {
		billId := bills[0].GetId()
		filename := "bill-" + hexutil.Encode(billId[:]) + ".json"
		return path.Join(outputDir, filename), nil
	} else {
		return path.Join(outputDir, "bills.json"), nil
	}
}

func newBillsDTO(bills ...*money.Bill) *block.Bills {
	var billsDTO []*block.Bill
	for _, b := range bills {
		billsDTO = append(billsDTO, b.ToProto())
	}
	return &block.Bills{Bills: billsDTO}
}

func (t *TrustBase) verify() error {
	if len(t.RootValidators) == 0 {
		return errors.New("missing trust base key info")
	}
	for _, rv := range t.RootValidators {
		if len(rv.SigningPublicKey) == 0 {
			return errors.New("missing trust base signing public key")
		}
		if len(rv.NodeIdentifier) == 0 {
			return errors.New("missing trust base node identifier")
		}
	}
	return nil
}

func (t *TrustBase) toVerifiers() (map[string]abcrypto.Verifier, error) {
	return genesis.NewValidatorTrustBase(t.RootValidators)
}
