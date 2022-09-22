package cmd

import (
	"context"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/errors"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/spf13/cobra"
)

func newNodeIdentifierCmd(ctx context.Context) *cobra.Command {
	var file string
	var cmd = &cobra.Command{
		Use:   "identifier",
		Short: "Returns the ID of the node",
		RunE: func(cmd *cobra.Command, args []string) error {
			return identifierRunFun(ctx, file)
		},
	}
	cmd.Flags().StringVarP(&file, keyFileCmdFlag, "k", "", "path to the key file")
	err := cmd.MarkFlagRequired(keyFileCmdFlag)
	if err != nil {
		panic(err)
	}
	return cmd
}

func identifierRunFun(_ context.Context, file string) error {
	keys, err := LoadKeys(file, false, false)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", file)
	}
	id, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", id)
	return nil

}
