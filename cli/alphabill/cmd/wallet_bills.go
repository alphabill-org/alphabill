package cmd

import (
	"fmt"
	"os"
	"path"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/money"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const (
	billIdCmdName          = "bill-id"
	billOrderNumberCmdName = "bill-order-number"
	outputPathCmdName      = "output-path"
)

var (
	errBillOrderNumberOutOfBounds = errors.New("bill order number out of bounds")
)

type (
	BillProofDTO struct {
		Id     []byte                `json:"id"`
		Value  uint64                `json:"value"`
		TxHash []byte                `json:"txHash"`
		Tx     *txsystem.Transaction `json:"tx"`
		Proof  *block.BlockProof     `json:"proof"`
	}
	BillProofsDTO struct {
		Bills []*BillProofDTO `json:"bills"`
	}
)

func newBillProofDTO(b *money.Bill) *BillProofDTO {
	b32 := b.Id.Bytes32()
	return &BillProofDTO{
		Id:     b32[:],
		Value:  b.Value,
		TxHash: b.TxHash,
		Tx:     b.Tx,
		Proof:  b.BlockProof,
	}
}

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
		Short: "lists bill ids",
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
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "specifies which account bill proofs to export")
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
		return exportBill(b, outputPath)
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
		return exportBill(b, outputPath)
	}
	// export all bills if neither --bill-id or --bill-order-number are given
	bills, err := w.GetBills(accountNumber - 1)
	if err != nil {
		return err
	}
	return exportBills(bills, outputPath)
}

func exportBill(b *money.Bill, outputPath string) error {
	billId := b.Id.Bytes32()
	filename := "bill-" + hexutil.Encode(billId[:]) + ".json"
	outputFile := path.Join(outputPath, filename)
	err := util.WriteJsonFile(outputFile, newBillProofDTO(b))
	if err != nil {
		return err
	}
	consoleWriter.Println("Exported bill to: " + outputFile)
	return nil
}

func exportBills(bills []*money.Bill, outputPath string) error {
	var billsDTO []*BillProofDTO
	for _, b := range bills {
		billsDTO = append(billsDTO, newBillProofDTO(b))
	}
	outputFile := path.Join(outputPath, "bills.json")
	err := util.WriteJsonFile(outputFile, &BillProofsDTO{Bills: billsDTO})
	if err != nil {
		return err
	}
	consoleWriter.Println("Exported bills to: " + outputFile)
	return nil
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
