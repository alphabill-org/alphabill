package cmd

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	abclient "github.com/alphabill-org/alphabill/pkg/client"
	vdclient "github.com/alphabill-org/alphabill/pkg/wallet/vd/client"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

const timeoutDelta = 100 // TODO make timeout configurable?

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

	vdCmd.AddCommand(regCmd(&wait))
	vdCmd.AddCommand(listBlocksCmd(&wait))

	return vdCmd
}

func regCmd(wait *bool) *cobra.Command {
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

			vdClient, err := initVDClient(cmd.Context(), cmd, wait, sync)
			if err != nil {
				return err
			}

			if hash != "" {
				err = vdClient.RegisterHash(hash)
			} else if file != "" {
				err = vdClient.RegisterFileHash(file)
			}
			return err
		},
	}
	cmd.Flags().StringP("hash", "d", "", "register data hash (hex with or without 0x prefix)")
	cmd.Flags().StringP("file", "f", "", "create sha256 hash of the file contents and register data hash")
	// cmd.MarkFlagsMutuallyExclusive("hash", "file") TODO use once 1.5.0 is released
	cmd.Flags().BoolVarP(&sync, "sync", "s", false, "synchronize ledger until block with given tx")

	return cmd
}

func listBlocksCmd(wait *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list-blocks",
		Short: "prints all non-empty blocks from the ledger",
		RunE: func(cmd *cobra.Command, args []string) error {
			vdClient, err := initVDClient(cmd.Context(), cmd, wait, false)
			if err != nil {
				return err
			}

			if err = vdClient.ListAllBlocksWithTx(); err != nil {
				return err
			}
			return nil
		},
	}
	return cmd
}

func initVDClient(ctx context.Context, cmd *cobra.Command, wait *bool, sync bool) (*vdclient.VDClient, error) {
	uri, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return nil, err
	}

	err = wlog.InitStdoutLogger(wlog.INFO)
	if err != nil {
		return nil, err
	}

	vdClient, err := vdclient.New(ctx, &vdclient.VDClientConfig{
		AbConf: &abclient.AlphabillClientConfig{
			Uri:          uri,
			WaitForReady: *wait,
		},
		WaitBlock:    sync,
		BlockTimeout: timeoutDelta})
	if err != nil {
		return nil, err
	}
	return vdClient, nil
}
