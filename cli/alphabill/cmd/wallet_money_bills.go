package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/spf13/cobra"
)

const (
	billIdCmdName          = "bill-id"
	billOrderNumberCmdName = "bill-order-number"
	outputPathCmdName      = "output-path"
	billFileCmdName        = "bill-file"
	trustBaseFileCmdName   = "trust-base-file"
)

var (
	errBillOrderNumberOutOfBounds = errors.New("bill order number out of bounds")
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
	cmd.AddCommand(importCmd(config))
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
	cmd.Flags().Uint64P(keyCmdName, "k", 0, "specifies which account bills to list (default: all accounts)")
	cmd.Flags().BoolP(showUnswappedCmdName, "s", false, "includes unswapped dust bills in output")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	showUnswapped, err := cmd.Flags().GetBool(showUnswappedCmdName)
	if err != nil {
		return err
	}

	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	type accountBillGroup struct {
		accountIndex uint64
		bills        []*money.Bill
	}
	var accountBillGroups []*accountBillGroup
	if accountNumber == 0 {
		bills, err := w.GetAllBills()
		if err != nil {
			return err
		}
		for accIdx, accBills := range bills {
			accountBillGroups = append(accountBillGroups, &accountBillGroup{accountIndex: uint64(accIdx), bills: accBills})
		}
	} else {
		accountIndex := accountNumber - 1
		accountBills, err := w.GetBills(accountIndex)
		if err != nil {
			return err
		}
		accountBillGroups = append(accountBillGroups, &accountBillGroup{accountIndex: accountIndex, bills: accountBills})
	}

	if !showUnswapped {
		for i, accBillGroup := range accountBillGroups {
			accountBillGroups[i].bills = filterDcBills(accBillGroup.bills)
		}
	}

	for _, group := range accountBillGroups {
		if len(group.bills) == 0 {
			consoleWriter.Println(fmt.Sprintf("Account #%d - empty", group.accountIndex+1))
		} else {
			consoleWriter.Println(fmt.Sprintf("Account #%d", group.accountIndex+1))
		}
		for j, bill := range group.bills {
			consoleWriter.Println(fmt.Sprintf("#%d 0x%X %d", j+1, bill.GetID(), bill.Value))
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
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies which account bills to export")
	cmd.Flags().BytesHexP(billIdCmdName, "b", nil, "bill ID in hex format (without 0x prefix)")
	cmd.Flags().IntP(billOrderNumberCmdName, "n", 0, "bill order number (from list command output)")
	cmd.Flags().StringP(outputPathCmdName, "o", "", "output directory for bills, directory is created if it does not exist (default: CWD)")
	return cmd
}

func execExportCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	billId, err := cmd.Flags().GetBytesHex(billIdCmdName)
	if err != nil {
		return err
	}
	billOrderNumber, err := cmd.Flags().GetInt(billOrderNumberCmdName)
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
	// create directories if output path dir does not exist
	err = os.MkdirAll(outputPath, 0700) // -rwx------
	if err != nil {
		return err
	}
	// export bill using --bill-id if present
	if len(billId) > 0 {
		b, err := w.GetBill(accountNumber-1, billId)
		if err != nil {
			return err
		}
		outputFile, err := writeBillsToFile(outputPath, b)
		if err != nil {
			return err
		}
		consoleWriter.Println("Exported bill(s) to: " + outputFile)
		return nil
	}
	// export bill using --bill-order-number if present
	if billOrderNumber > 0 {
		bills, err := w.GetBills(accountNumber - 1)
		if err != nil {
			return err
		}
		bills = filterDcBills(bills)
		billIndex := billOrderNumber - 1
		if billIndex >= len(bills) {
			return errBillOrderNumberOutOfBounds
		}
		b := bills[billIndex]
		outputFile, err := writeBillsToFile(outputPath, b)
		if err != nil {
			return err
		}
		consoleWriter.Println("Exported bill(s) to: " + outputFile)
		return nil
	}
	// export all bills if neither --bill-id or --bill-order-number are given
	bills, err := w.GetBills(accountNumber - 1)
	if err != nil {
		return err
	}
	outputFile, err := writeBillsToFile(outputPath, bills...)
	if err != nil {
		return err
	}
	consoleWriter.Println("Exported bill(s) to: " + outputFile)
	return nil
}

func importCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "imports bills to wallet",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execImportCmd(cmd, config)
		},
		Hidden: true,
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to import the bills")
	cmd.Flags().StringP(billFileCmdName, "b", "", "path to bill file (any file from export command output)")
	cmd.Flags().StringP(trustBaseFileCmdName, "t", "", "path to trust base file")
	err := cmd.MarkFlagRequired(billFileCmdName)
	if err != nil {
		return nil
	}
	err = cmd.MarkFlagRequired(trustBaseFileCmdName)
	if err != nil {
		return nil
	}
	return cmd
}

func execImportCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	billFile, err := cmd.Flags().GetString(billFileCmdName)
	if err != nil {
		return err
	}
	trustBaseFile, err := cmd.Flags().GetString(trustBaseFileCmdName)
	if err != nil {
		return err
	}
	trustBase, err := util.ReadJsonFile(trustBaseFile, &TrustBase{})
	if err != nil {
		return err
	}
	err = trustBase.verify()
	if err != nil {
		return err
	}
	verifiers, err := trustBase.toVerifiers()
	if err != nil {
		return err
	}
	billFileJson, err := bp.ReadBillsFile(billFile)
	if err != nil {
		return err
	}
	if len(billFileJson.Bills) == 0 {
		return errors.New("bill file does not contain any bills")
	}
	txConverter := money.NewTxConverter(w.SystemID())
	err = billFileJson.Verify(txConverter, verifiers)
	if err != nil {
		return err
	}
	for _, b := range billFileJson.Bills {
		err = w.AddBill(accountNumber-1, newBill(b))
		if err != nil {
			return err
		}
	}
	consoleWriter.Println("Successfully imported bill(s).")
	return nil
}

// writeBillsToFile writes bill(s) to given directory.
// Creates outputDir if it does not already exist. Returns output file.
func writeBillsToFile(outputDir string, bills ...*money.Bill) (string, error) {
	outputFile, err := getOutputFile(outputDir, bills)
	if err != nil {
		return "", err
	}
	err = os.MkdirAll(outputDir, 0700) // -rwx------
	if err != nil {
		return "", err
	}
	err = bp.WriteBillsFile(outputFile, newBillsDTO(bills...))
	if err != nil {
		return "", err
	}
	return outputFile, nil
}

// getOutputFile returns filename either bill-<bill-id-hex>.json or bills.json
func getOutputFile(outputDir string, bills []*money.Bill) (string, error) {
	if len(bills) == 0 {
		return "", errors.New("no bills to export")
	} else if len(bills) == 1 {
		billId := bills[0].GetID()
		filename := "bill-" + hexutil.Encode(billId[:]) + ".json"
		return path.Join(outputDir, filename), nil
	} else {
		return path.Join(outputDir, "bills.json"), nil
	}
}

func filterDcBills(bills []*money.Bill) []*money.Bill {
	var normalBills []*money.Bill
	for _, b := range bills {
		if !b.IsDcBill {
			normalBills = append(normalBills, b)
		}
	}
	return normalBills
}

func newBillsDTO(bills ...*money.Bill) *bp.Bills {
	var billsDTO []*bp.Bill
	for _, b := range bills {
		billsDTO = append(billsDTO, b.ToProto())
	}
	return &bp.Bills{Bills: billsDTO}
}

func newBill(b *bp.Bill) *money.Bill {
	return &money.Bill{
		Id:         uint256.NewInt(0).SetBytes(b.Id),
		Value:      b.Value,
		TxHash:     b.TxHash,
		IsDcBill:   b.IsDcBill,
		BlockProof: newBlockProof(b.TxProof),
	}
}

func newBlockProof(b *block.TxProof) *money.BlockProof {
	return &money.BlockProof{
		Tx:          b.Tx,
		BlockNumber: b.BlockNumber,
		Proof:       b.Proof,
	}
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
