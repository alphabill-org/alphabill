package cmd

import (
	"crypto"
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
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
	moneySystemIdentifier         = []byte{0, 0, 0, 0}
)

type (
	// BillsDTO json schema for bill import and export.
	BillsDTO struct {
		Bills []*BillDTO `json:"bills"`
	}
	// BillDTO individual bill struct in import/export schema. All fields mandatory.
	BillDTO struct {
		Id     []byte                `json:"id"`
		Value  uint64                `json:"value"`
		TxHash []byte                `json:"txHash"`
		Tx     *txsystem.Transaction `json:"tx"`
		Proof  *block.BlockProof     `json:"proof"`
	}
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
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies which account bills to list")
	return cmd
}

func execListCmd(cmd *cobra.Command, config *walletConfig) error {
	w, err := loadExistingWallet(cmd, config.WalletHomeDir, "")
	if err != nil {
		return err
	}
	defer w.Shutdown()

	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}

	bills, err := w.GetBills(accountNumber - 1)
	if err != nil {
		return err
	}
	bills = filterDcBills(bills)
	if len(bills) == 0 {
		consoleWriter.Println("Wallet is empty.")
		return nil
	}
	for i, b := range bills {
		consoleWriter.Println(fmt.Sprintf("#%d 0x%X %d", i+1, b.Id.Bytes32(), b.Value))
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
	}
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies to which account to import the bills")
	cmd.Flags().StringP(billFileCmdName, "b", "", "path to bill file (any file from export command output)")
	cmd.Flags().StringP(trustBaseFileCmdName, "t", "", "path to trust base file")
	_ = cmd.MarkFlagRequired(billFileCmdName)
	_ = cmd.MarkFlagRequired(trustBaseFileCmdName)
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
	billFileJson, err := util.ReadJsonFile(billFile, &BillsDTO{})
	if err != nil {
		return err
	}
	if len(billFileJson.Bills) == 0 {
		return errors.New("bill file does not contain any bills")
	}
	for _, b := range billFileJson.Bills {
		err = b.verifyProof(verifiers)
		if err != nil {
			return err
		}
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
	err = util.WriteJsonFile(outputFile, newBillsDTO(bills...))
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
		billId := bills[0].Id.Bytes32()
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

func newBillDTO(b *money.Bill) *BillDTO {
	b32 := b.Id.Bytes32()
	return &BillDTO{
		Id:     b32[:],
		Value:  b.Value,
		TxHash: b.TxHash,
		Tx:     b.Tx,
		Proof:  b.BlockProof,
	}
}

func newBillsDTO(bills ...*money.Bill) *BillsDTO {
	var billsDTO []*BillDTO
	for _, b := range bills {
		billsDTO = append(billsDTO, newBillDTO(b))
	}
	return &BillsDTO{Bills: billsDTO}
}

func newBill(b *BillDTO) *money.Bill {
	return &money.Bill{
		Id:         uint256.NewInt(0).SetBytes(b.Id),
		Value:      b.Value,
		TxHash:     b.TxHash,
		Tx:         b.Tx,
		BlockProof: b.Proof,
	}
}

func (b *BillDTO) verifyProof(verifiers map[string]abcrypto.Verifier) error {
	if b.Proof == nil {
		return errors.New("proof is nil")
	}
	tx, err := moneytx.NewMoneyTx(moneySystemIdentifier, b.Tx)
	if err != nil {
		return err
	}
	return b.Proof.Verify(tx, verifiers, crypto.SHA256)
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
