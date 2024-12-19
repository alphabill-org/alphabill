package cmd

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

func newNodeIDCmd() *cobra.Command {
	var file string
	var cmd = &cobra.Command{
		Use:   "identifier",
		Short: "Returns the ID of the node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return identifierRunFun(cmd.Context(), file)
		},
	}
	cmd.Flags().StringVarP(&file, keyFileCmdFlag, "k", "", "path to the key file")
	if err := cmd.MarkFlagRequired(keyFileCmdFlag); err != nil {
		panic(err)
	}
	return cmd
}

func identifierRunFun(_ context.Context, file string) error {
	keys, err := LoadKeys(file, false, false)
	if err != nil {
		return fmt.Errorf("failed to load keys %v: %w", file, err)
	}
	id, err := peer.IDFromPublicKey(keys.AuthPrivKey.GetPublic())
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", id)
	return nil

}
