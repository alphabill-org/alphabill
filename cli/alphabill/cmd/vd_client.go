package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	vdclient "github.com/alphabill-org/alphabill/pkg/wallet/vd"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const timeoutDelta = 10 // TODO make timeout configurable?

func vdCmd(config *walletConfig) *cobra.Command {
	var vdCmd = &cobra.Command{
		Use:   "vd",
		Short: "submit data to Verifiable Data partition",
	}

	var wait bool
	vdCmd.PersistentFlags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill uri to connect to")
	vdCmd.PersistentFlags().BoolVarP(&wait, "wait", "w", false, "wait until server is available")
	err := vdCmd.PersistentFlags().MarkHidden("wait")
	if err != nil {
		return nil
	}

	vdCmd.AddCommand(regCmd(config, &wait))
	vdCmd.AddCommand(listBlocksCmd(config, &wait))

	return vdCmd
}

func regCmd(config *walletConfig, wait *bool) *cobra.Command {
	var sync bool
	cmd := &cobra.Command{
		Use:   "register",
		Short: "registers a new hash value on the ledger",
		RunE: func(cmd *cobra.Command, args []string) error {
			hash, err := cmd.Flags().GetString("hash")
			if err != nil {
				return err
			}
			file, err := cmd.Flags().GetString("file")
			if err != nil {
				return err
			}
			if hash != "" && file != "" {
				return errors.New("'hash' and 'file' flags are mutually exclusive")
			}
			accountKey, err := loadAccountKey(cmd, config)
			if err != nil {
				return err
			}
			vdClient, err := initVDClient(cmd, config.WalletHomeDir, accountKey, wait, sync)
			if err != nil {
				return err
			}
			defer vdClient.Close()

			if hash != "" {
				err = vdClient.RegisterHash(cmd.Context(), hash)
			} else if file != "" {
				err = vdClient.RegisterFileHash(cmd.Context(), file)
			}
			return err
		},
	}
	cmd.Flags().StringP("hash", "d", "", "register data hash (hex with or without 0x prefix)")
	cmd.Flags().StringP("file", "f", "", "create sha256 hash of the file contents and register data hash")
	// cmd.MarkFlagsMutuallyExclusive("hash", "file") TODO use once 1.5.0 is released
	cmd.Flags().Uint64P(keyCmdName, "k", 1, "the account number used to pay fees")
	cmd.Flags().BoolVarP(&sync, "sync", "s", false, "synchronize ledger until block with given tx")

	return cmd
}

func listBlocksCmd(config *walletConfig, wait *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-blocks",
		Short: "prints all non-empty blocks from the ledger",
		RunE: func(cmd *cobra.Command, args []string) error {
			vdClient, err := initVDClient(cmd, config.WalletHomeDir, nil, wait, false)
			if err != nil {
				return err
			}
			defer vdClient.Close()

			if err = vdClient.ListAllBlocksWithTx(cmd.Context()); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func initVDClient(cmd *cobra.Command, walletHomeDir string, accountKey *account.AccountKey, wait *bool, confirmTx bool) (*vdclient.VDClient, error) {
	vdNodeURL, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return nil, err
	}
	err = wlog.InitStdoutLogger(wlog.INFO)
	if err != nil {
		return nil, err
	}

	vdClient, err := vdclient.New(&vdclient.VDClientConfig{
		VDNodeURL:         vdNodeURL,
		WaitForReady:      *wait,
		ConfirmTx:         confirmTx,
		ConfirmTxTimeout:  timeoutDelta,
		AccountKey:        accountKey,
		WalletHomeDir:     walletHomeDir,
	})
	if err != nil {
		return nil, err
	}
	return vdClient, nil
}

func loadAccountKey(cmd *cobra.Command, config *walletConfig) (*account.AccountKey, error) {
	accountNumber, err := cmd.Flags().GetUint64(keyCmdName)
	if err != nil {
		return nil, err
	}
	am, err := loadExistingAccountManager(cmd, config.WalletHomeDir)
	if err != nil {
		return nil, err
	}
	defer am.Close()

	return am.GetAccountKey(accountNumber - 1)
}
