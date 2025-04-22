package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type nodeIDFlags struct {
	*baseFlags
	keyConfFlags
}

func newNodeIDCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &nodeIDFlags{baseFlags: baseFlags}

	var cmd = &cobra.Command{
		Use:   "node-id",
		Short: "Extracts nodeID from key configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return extractNodeID(cmd.Context(), flags)
		},
	}
	flags.addKeyConfFlags(cmd, false)
	return cmd
}

func extractNodeID(_ context.Context, flags *nodeIDFlags) error {
	keyConf, err := flags.loadKeyConf(flags.baseFlags, false)
	if err != nil {
		return err
	}
	nodeID, err := keyConf.NodeID()
	if err != nil {
		return err
	}
	fmt.Printf("%s\n", nodeID)
	return nil
}
