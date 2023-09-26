package cmd

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	evmwallet "github.com/alphabill-org/alphabill/pkg/wallet/evm"
	evmclient "github.com/alphabill-org/alphabill/pkg/wallet/evm/client"
	"github.com/ethereum/go-ethereum/common"
	"github.com/spf13/cobra"
)

const (
	defaultEvmNodeRestURL = "localhost:29866"
	dataCmdName           = "data"
	maxGasCmdName         = "max-gas"
	valueCmdName          = "value"
	scSizeLimit24Kb       = 24 * 1024
	defaultEvmAddrLen     = 20
)

func evmCmd(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "evm",
		Short: "interact with alphabill EVM partition",
	}
	cmd.AddCommand(evmCmdDeploy(config))
	cmd.AddCommand(evmCmdExecute(config))
	cmd.AddCommand(evmCmdCall(config))
	cmd.AddCommand(evmCmdBalance(config))
	cmd.PersistentFlags().StringP(alphabillApiURLCmdName, "r", defaultEvmNodeRestURL, "alphabill EVM partition node REST URI to connect to")
	return cmd
}

func evmCmdDeploy(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "deploys a new smart contract on evm partition by sending a transaction on the block chain",
		Long: "Executes smart contract deployment by sending a transaction on the block chain." +
			"On success the new smart contract address is printed as result and it can be used to execute/call smart contract functions",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdDeploy(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	// data - smart contract code
	cmd.Flags().String(dataCmdName, "", "contract code as hex string")
	// max-gas
	cmd.Flags().Uint64(maxGasCmdName, 0, "maximum amount of gas user is willing to spend")
	if err := cmd.MarkFlagRequired(dataCmdName); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(maxGasCmdName); err != nil {
		return nil
	}
	return cmd
}

func evmCmdExecute(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "execute",
		Short: "executes smart contract call by sending a transaction on the block chain",
		Long: "Executes smart contract call by sending a transaction on the block chain." +
			"State changes are persisted and result is stored in block chain",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdExecute(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for sending the transaction")
	// to address - smart contract to call
	cmd.Flags().String(addressCmdName, "", "smart contract address in hexadecimal format, must start with 0x and be 20 characters in length")
	// data - function ID + parameter
	cmd.Flags().String(dataCmdName, "", "4 byte function ID and optionally argument in hex")
	// max amount of gas user is willing to spend
	cmd.Flags().Uint64(maxGasCmdName, 0, "maximum amount of gas user is willing to spend")
	if err := cmd.MarkFlagRequired(addressCmdName); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(dataCmdName); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(maxGasCmdName); err != nil {
		return nil
	}
	return cmd
}

func evmCmdCall(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "call",
		Short: "executes a smart contract call immediately without creating a transaction on the block chain",
		Long: "Executes a smart contract call immediately without creating a transaction on the block chain." +
			"State changes are not persisted and nothing is added to the block. Often used for executing read-only smart contract functions.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdCall(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for from address in evm call")
	// to address - smart contract to call
	cmd.Flags().String(addressCmdName, "", "to address in hexadecimal format, must be 20 characters in length")
	// data
	cmd.Flags().String(dataCmdName, "", "data as hex string")
	// max amount of gas user is willing to spend
	cmd.Flags().Uint64(maxGasCmdName, 50000000, "(optional) maximum amount of gas user is willing to spend")
	// value, default 0
	cmd.Flags().Uint64(valueCmdName, 0, "(optional) value to transfer")
	_ = cmd.Flags().MarkHidden(valueCmdName)
	if err := cmd.MarkFlagRequired(addressCmdName); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(dataCmdName); err != nil {
		return nil
	}
	if err := cmd.MarkFlagRequired(maxGasCmdName); err != nil {
		return nil
	}
	return cmd
}

func evmCmdBalance(config *walletConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:    "balance",
		Short:  "get account balance",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return execEvmCmdBalance(cmd, config)
		},
	}
	// account from which to call - pay for the transaction
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "which key to use for balance")
	return cmd
}

func initEvmWallet(cmd *cobra.Command, config *walletConfig) (*evmwallet.Wallet, error) {
	uri, err := cmd.Flags().GetString(alphabillApiURLCmdName)
	if err != nil {
		return nil, err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return nil, err
	}
	wallet, err := evmwallet.New(evm.DefaultEvmTxSystemIdentifier, uri, am)
	if err != nil {
		return nil, err
	}
	return wallet, nil
}

// readHexFlag returns nil in case array is empty (weird behaviour by cobra)
func readHexFlag(cmd *cobra.Command, flag string) ([]byte, error) {
	str, err := cmd.Flags().GetString(flag)
	if err != nil {
		return nil, err
	}
	if len(str) == 0 {
		return nil, fmt.Errorf("argument is empty")
	}
	res, err := hex.DecodeString(str)
	if err != nil {
		return nil, fmt.Errorf("hex decode error: %w", err)
	}
	return res, err
}

func execEvmCmdDeploy(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	code, err := readHexFlag(cmd, dataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", dataCmdName, err)
	}
	if len(code) > scSizeLimit24Kb {
		return fmt.Errorf("contract code too big, maximum size is 24Kb")
	}
	maxGas, err := cmd.Flags().GetUint64(maxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", maxGasCmdName, err)
	}
	attributes := &evmclient.TxAttributes{
		Data: code,
		Gas:  maxGas,
	}
	result, err := w.SendEvmTx(cmd.Context(), accountNumber, attributes)
	if err != nil {
		return fmt.Errorf("excution error %w", err)
	}
	printResult(result)
	return nil
}

func execEvmCmdExecute(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	// get to address
	toAddr, err := readHexFlag(cmd, addressCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", addressCmdName, err)
	}
	if len(toAddr) != defaultEvmAddrLen {
		return fmt.Errorf("invalid address %x, address must be 20 bytes", toAddr)
	}
	// read binary contract file
	fnIDAndArg, err := readHexFlag(cmd, dataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", dataCmdName, err)
	}
	maxGas, err := cmd.Flags().GetUint64(maxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", maxGasCmdName, err)
	}
	attributes := &evmclient.TxAttributes{
		To:   toAddr,
		Data: fnIDAndArg,
		Gas:  maxGas,
	}
	result, err := w.SendEvmTx(cmd.Context(), accountNumber, attributes)
	if err != nil {
		return fmt.Errorf("excution error %w", err)
	}
	printResult(result)
	return nil
}

func execEvmCmdCall(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	// get to address
	toAddr, err := readHexFlag(cmd, addressCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", addressCmdName, err)
	}
	if len(toAddr) != defaultEvmAddrLen {
		return fmt.Errorf("invalid address %x, address must be 20 bytes", toAddr)
	}
	// data
	data, err := readHexFlag(cmd, dataCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", dataCmdName, err)
	}
	if len(data) > scSizeLimit24Kb {
		return fmt.Errorf("")
	}
	maxGas, err := cmd.Flags().GetUint64(maxGasCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", maxGasCmdName, err)
	}
	value, err := cmd.Flags().GetUint64(valueCmdName)
	if err != nil {
		return fmt.Errorf("failed to read '%s' parameter: %w", valueCmdName, err)
	}
	attributes := &evmclient.CallAttributes{
		To:    toAddr,
		Data:  data,
		Value: new(big.Int).SetUint64(value),
		Gas:   maxGas,
	}
	result, err := w.EvmCall(cmd.Context(), accountNumber, attributes)
	if err != nil {
		return fmt.Errorf("excution error %w", err)
	}
	printResult(result)
	return nil
}

func execEvmCmdBalance(cmd *cobra.Command, config *walletConfig) error {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return fmt.Errorf("key parameter read failed: %w", err)
	}
	w, err := initEvmWallet(cmd, config)
	if err != nil {
		return fmt.Errorf("evm wallet init failed: %w", err)
	}
	defer w.Shutdown()
	balance, err := w.GetBalance(cmd.Context(), accountNumber)
	if err != nil {
		return fmt.Errorf("balance error %w", err)
	}
	inAlpha := evmwallet.ConvertBalanceToAlpha(balance)
	balanceStr := amountToString(inAlpha, 8)
	balanceEthStr := amountToString(balance.Uint64(), 18)
	consoleWriter.Println(fmt.Sprintf("#%d %s (eth: %s)", accountNumber, balanceStr, balanceEthStr))
	return nil
}

func execEvmCmdAddress(cmd *cobra.Command, config *walletConfig) error {
	if cmd.Flags().Changed("public-key") {
		toAddr, err := readHexFlag(cmd, "public-key")
		if err != nil {
			return fmt.Errorf("failed to read 'public-key' parameter: %w", err)
		}
		if addr, err := evmwallet.GenerateAddress(toAddr); err == nil {
			consoleWriter.Println(fmt.Sprintf("Public-key addr: %s", addr.String()))
		}
		return nil
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return fmt.Errorf("account manager init failed: %w", err)
	}
	defer am.Close()
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return err
	}
	if accountNumber != 0 {
		accountIndex := accountNumber - 1
		key, err := am.GetAccountKey(accountIndex)
		if err != nil {
			return err
		}
		if addr, err := evmwallet.GenerateAddress(key.PubKey); err == nil {
			consoleWriter.Println(fmt.Sprintf("Account #%d %s", accountNumber, addr.String()))
		}
		return nil
	}
	// Print addresses of all keys
	pubKeys, err := am.GetPublicKeys()
	if err != nil {
		return err
	}
	for accountIndex, key := range pubKeys {
		accNum := accountIndex + 1
		if addr, err := evmwallet.GenerateAddress(key); err == nil {
			consoleWriter.Println(fmt.Sprintf("Account #%d %s", accNum, addr.String()))
		}
	}
	return nil
}

func printResult(result *evmclient.Result) {
	if !result.Success {
		consoleWriter.Println(fmt.Sprintf("Evm transaction failed: %s", result.Details.ErrorDetails))
		consoleWriter.Println(fmt.Sprintf("Evm transaction processing fee: %v", amountToString(result.ActualFee, 8)))
		return
	}
	consoleWriter.Println(fmt.Sprintf("Evm transaction succeeded"))
	consoleWriter.Println(fmt.Sprintf("Evm transaction processing fee: %v", amountToString(result.ActualFee, 8)))
	noContract := common.Address{} // content if no contract is deployed
	if result.Details.ContractAddr != noContract {
		consoleWriter.Println(fmt.Sprintf("Deployed smart contract address: %x", result.Details.ContractAddr))
	}
	for i, l := range result.Details.Logs {
		consoleWriter.Println(fmt.Sprintf("Evm log %v : %v", i, l))
	}
	if len(result.Details.ReturnData) > 0 {
		consoleWriter.Println(fmt.Sprintf("Evm execution returned: %X", result.Details.ReturnData))
	}
}
