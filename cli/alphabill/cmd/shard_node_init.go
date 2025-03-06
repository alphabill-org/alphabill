package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

)

const nodeInfoFileName = "node-info.json"

type shardNodeInitFlags struct {
	*baseFlags
	keyConfFlags
}

func shardNodeInitCmd(baseFlags *baseFlags) *cobra.Command {
	cmdFlags := &shardNodeInitFlags{baseFlags: baseFlags}

	var cmd = &cobra.Command{
		Use:   "init",
		Short: "Generate public node info from keys",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shardNodeInit(cmd.Context(), cmdFlags)
		},
	}

	cmdFlags.addKeyConfFlags(cmd, true)
	return cmd
}

func shardNodeInit(_ context.Context, flags *shardNodeInitFlags) error {
	nodeInfoPath := flags.pathWithDefault("", nodeInfoFileName)
	if util.FileExists(nodeInfoPath) {
		return fmt.Errorf("node info %q already exists", nodeInfoPath)
	} else if err := os.MkdirAll(filepath.Dir(nodeInfoPath), 0700); err != nil {  // -rwx------
		return err
	}
	keyConf, err := flags.loadKeyConf(flags.baseFlags, flags.Generate)
	if err != nil {
		return err
	}
	nodeID, err := keyConf.NodeID()
	if err != nil {
		return fmt.Errorf("failed to get node identifier: %w", err)
	}
	signer, err := keyConf.Signer()
	if err != nil {
		return err
	}
	sigVerifier, err := signer.Verifier()
	if err != nil {
		return fmt.Errorf("failed to create verifier for sigKey: %w", err)
	}
	sigKey, err := sigVerifier.MarshalPublicKey()
	if err != nil {
		return fmt.Errorf("failed to marshal sigKey: %w", err)
	}
	nodeInfo := &types.NodeInfo{
		NodeID: nodeID.String(),
		SigKey: sigKey,
		Stake:  1,
	}

	return util.WriteJsonFile(nodeInfoPath, nodeInfo)
}
