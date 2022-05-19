package cmd

import (
	"context"
	"errors"
	"fmt"

	vd "gitdc.ee.guardtime.com/alphabill/alphabill/pkg/vd"

	"github.com/spf13/cobra"
)

func newVDClientCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	var vdCmd = &cobra.Command{
		Use:   "vd",
		Short: "cli for submitting data to Verifiable Data partition",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return initializeConfig(cmd, baseConfig)
		},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Error: must specify a subcommand")
		},
	}
	vdCmd.AddCommand(regCmd(ctx, baseConfig))

	return vdCmd
}

func regCmd(ctx context.Context, _ *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "register",
		RunE: func(cmd *cobra.Command, args []string) error {
			uri, err := cmd.Flags().GetString(alphabillUriCmdName)
			if err != nil {
				return err
			}
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

			vdClient := vd.New(ctx, &vd.AlphabillClientConfig{
				Uri: uri,
			})

			if hash != "" {
				return vdClient.RegisterHash(hash)
			} else if file != "" {
				return vdClient.RegisterFileHash(file)
			}
			log.Info("Done.")
			return nil
		},
	}
	cmd.Flags().StringP("hash", "d", "", "register data hash (hex prefixed with 0x)")
	cmd.Flags().StringP("file", "f", "", "create sha256 hash of the file contents and register data hash")
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	// cmd.MarkFlagsMutuallyExclusive("hash", "file") TODO use once 1.5.0 is released

	return cmd
}
