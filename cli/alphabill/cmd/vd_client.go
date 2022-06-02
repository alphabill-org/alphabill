package cmd

import (
	"context"
	"errors"
	"fmt"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"

	vd "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/vd"
	wlog "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"
	"github.com/spf13/cobra"
)

const timeoutDelta = 100 // TODO make timeout configurable?

func newVDClientCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	var vdCmd = &cobra.Command{
		Use:   "vd-client",
		Short: "cli for submitting data to Verifiable Data partition",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd, baseConfig)
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Error: must specify a subcommand")
		},
	}

	var wait bool
	vdCmd.PersistentFlags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	vdCmd.PersistentFlags().BoolVarP(&wait, "wait", "w", false, "wait until server is available")
	err := vdCmd.PersistentFlags().MarkHidden("wait")
	if err != nil {
		return nil
	}

	vdCmd.AddCommand(regCmd(ctx, baseConfig, &wait))
	vdCmd.AddCommand(listBlocksCmd(ctx, baseConfig, &wait))

	return vdCmd
}

func regCmd(ctx context.Context, _ *baseConfiguration, wait *bool) *cobra.Command {
	var sync bool
	cmd := &cobra.Command{
		Use: "register",
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

			vdClient, err := initVDClient(ctx, cmd, wait, sync)
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

func listBlocksCmd(ctx context.Context, _ *baseConfiguration, wait *bool) *cobra.Command {
	cmd := &cobra.Command{
		Use: "list-blocks",
		RunE: func(cmd *cobra.Command, args []string) error {
			vdClient, err := initVDClient(ctx, cmd, wait, false)
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

func initVDClient(ctx context.Context, cmd *cobra.Command, wait *bool, sync bool) (*vd.VDClient, error) {
	uri, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return nil, err
	}

	err = wlog.InitDefaultLogger()
	if err != nil {
		return nil, err
	}

	vdClient, err := vd.New(ctx, &vd.VDClientConfig{
		AbConf: &client.AlphabillClientConfig{
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
