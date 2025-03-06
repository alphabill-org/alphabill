package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/spf13/cobra"
)

const trustBaseFileName = "trust-base.json"

type (
	trustBaseFlags struct {
		TrustBaseFile string
	}

	trustBaseGenerateFlags struct {
		*baseFlags

		NetworkID       uint16
		NodeInfoFiles   []string // paths to node info files
		QuorumThreshold uint64   // optional custom quorum threshold (default len(nodes)*2/3 + 1)
	}

	trustBaseSignFlags struct {
		*baseFlags
		keyConfFlags
		trustBaseFlags
	}
)

func newTrustBaseCmd(baseConfig *baseFlags) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "trust-base",
		Short: "Tools to work with trust base files",
	}
	cmd.AddCommand(trustBaseGenerateCmd(baseConfig))
	cmd.AddCommand(trustBaseSignCmd(baseConfig))
	return cmd
}

func trustBaseGenerateCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &trustBaseGenerateFlags{baseFlags: baseFlags}

	var cmd = &cobra.Command{
		Use:   "generate",
		Short: "Generates a new trust base from node-info files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return trustBaseGenerate(flags)
		},
	}

	cmd.Flags().Uint16Var(&flags.NetworkID, "network-id", 0, "network identifier")
	cmd.Flags().StringSliceVarP(&flags.NodeInfoFiles, "node-info", "n", []string{}, "path to node info files")
	if err := cmd.MarkFlagRequired("node-info"); err != nil {
		panic(err)
	}
	cmd.Flags().Uint64Var(&flags.QuorumThreshold, "quorum-threshold", 0, "define custom quorum threshold (default: len(nodes)*2/3+1")
	return cmd
}

func trustBaseGenerate(flags *trustBaseGenerateFlags) error {
	if err := os.MkdirAll(flags.HomeDir, 0700); err != nil { // -rwx------
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	nodes, err := loadNodeInfoFiles(flags.NodeInfoFiles)
	if err != nil {
		return fmt.Errorf("failed to read node info files: %w", err)
	}

	trustBase, err := types.NewTrustBaseGenesis(types.NetworkID(flags.NetworkID), nodes,
		types.WithQuorumThreshold(flags.QuorumThreshold))
	if err != nil {
		return fmt.Errorf("failed to generate trust base: %w", err)
	}

	outputFile := filepath.Join(flags.HomeDir, trustBaseFileName)
	if err = util.WriteJsonFile(outputFile, trustBase); err != nil {
		return fmt.Errorf("failed to save '%s': %w", outputFile, err)
	}
	return nil
}

func trustBaseSignCmd(baseFlags *baseFlags) *cobra.Command {
	flags := &trustBaseSignFlags{baseFlags: baseFlags}
	var cmd = &cobra.Command{
		Use:   "sign",
		Short: "Sign a trust base file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return trustBaseSign(flags)
		},
	}
	flags.addKeyConfFlags(cmd, false)
	flags.addTrustBaseFlags(cmd)
	return cmd
}

func trustBaseSign(flags *trustBaseSignFlags) error {
	keyConf, err := flags.loadKeyConf(flags.baseFlags, false)
	if err != nil {
		return err
	}
	nodeID, err := keyConf.NodeID()
	if err != nil {
		return fmt.Errorf("failed to get node id: %w", err)
	}
	signer, err := keyConf.Signer()
	if err != nil {
		return err
	}
	trustBase, err := flags.loadTrustBase(flags.baseFlags)
	if err != nil {
		return err
	}

	// sign trust base
	if err = trustBase.Sign(nodeID.String(), signer); err != nil {
		return fmt.Errorf("failed to sign trust base: %w", err)
	}

	// write trust base
	if err = util.WriteJsonFile(flags.trustBasePath(flags.baseFlags), trustBase); err != nil {
		return fmt.Errorf("failed to save trust base: %w", err)
	}
	return nil
}

func loadNodeInfoFiles(paths []string) ([]*types.NodeInfo, error) {
	var nodes []*types.NodeInfo
	for _, p := range paths {
		node, err := util.ReadJsonFile(p, &types.NodeInfo{})
		if err != nil {
			return nil, fmt.Errorf("failed to read node info from '%s': %w", p, err)
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func (f *trustBaseFlags) addTrustBaseFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&f.TrustBaseFile, "trust-base", "t", "",
		fmt.Sprintf("path to trust base (default: %s)", filepath.Join("$AB_HOME", trustBaseFileName)))
}

func (f *trustBaseFlags) trustBasePath(baseFlags *baseFlags) string {
	return baseFlags.pathWithDefault(f.TrustBaseFile, trustBaseFileName)
}

func (f *trustBaseFlags) loadTrustBase(baseFlags *baseFlags) (ret *types.RootTrustBaseV1, err error) {
	return ret, baseFlags.loadConf(f.TrustBaseFile, trustBaseFileName, &ret)
}
