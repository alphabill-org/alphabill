package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	rootTrustBaseFileName = "root-trust-base.json"
)

type (
	genTrustBaseConfig struct {
		Base *baseConfiguration

		RootGenesisFiles []string // paths to root genesis record json files
		OutputDir        string   // path to output directory where root-trust-base.json file will be created (default $AB_HOME)
		QuorumThreshold  uint64   // optional custom quorum threshold, default: len(nodes)*2/3 + 1
	}

	signTrustBaseConfig struct {
		Base *baseConfiguration

		Keys          *keysConfig // config where to find the signing key
		TrustBaseFile string      // path to root-trust-base.json file that will be signed
	}
)

func (c *genTrustBaseConfig) getOutputDir() string {
	if c.OutputDir != "" {
		return c.OutputDir
	}
	return c.Base.HomeDir
}

func (c *signTrustBaseConfig) getTrustBaseFile() string {
	if c.TrustBaseFile != "" {
		return c.TrustBaseFile
	}
	return filepath.Join(c.Base.HomeDir, rootTrustBaseFileName)
}

func genTrustBaseCmd(config *rootGenesisConfig) *cobra.Command {
	trustBaseCfg := &genTrustBaseConfig{Base: config.Base}
	var cmd = &cobra.Command{
		Use:   "gen-trust-base",
		Short: "Generates root chain trust base file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return genTrustBaseRunFunc(trustBaseCfg)
		},
	}
	cmd.Flags().StringVarP(&trustBaseCfg.OutputDir, "output", "o", "", "path to output directory (default: $AB_HOME)")
	cmd.Flags().StringSliceVar(&trustBaseCfg.RootGenesisFiles, rootGenesisCmdName, []string{}, "path to combined root node genesis file(s)")
	if err := cmd.MarkFlagRequired(rootGenesisCmdName); err != nil {
		return nil
	}
	cmd.Flags().Uint64Var(&trustBaseCfg.QuorumThreshold, "quorum-threshold", 0, "define custom quorum threshold (default: len(nodes)*2/3+1")
	return cmd
}

func genTrustBaseRunFunc(config *genTrustBaseConfig) error {
	// load root genesis files
	rgs, err := loadRootGenesisFiles(config.RootGenesisFiles)
	if err != nil {
		return fmt.Errorf("failed to read root genesis files: %w", err)
	}
	// create trust base from each root genesis file, and verify they match
	var trustBase *types.RootTrustBaseV1
	var trustBaseBytes []byte
	for _, rg := range rgs {
		tb, err := rg.GenerateTrustBase(types.WithQuorumThreshold(config.QuorumThreshold))
		if err != nil {
			return fmt.Errorf("failed to generate trust base from root genesis: %w", err)
		}
		if trustBaseBytes == nil {
			trustBaseBytes, err = tb.SigBytes()
			if err != nil {
				return fmt.Errorf("failed to marshal first trust base: %w", err)
			}
			trustBase = tb
			continue
		}
		currentTbBytes, err := tb.SigBytes()
		if err != nil {
			return fmt.Errorf("failed to marshal trust base: %w", err)
		}
		if !bytes.Equal(trustBaseBytes, currentTbBytes) {
			return errors.New("generated trust base files from each root genesis file does not match")
		}
	}

	trustBaseFile := filepath.Join(config.getOutputDir(), rootTrustBaseFileName)
	if err = util.WriteJsonFile(trustBaseFile, &trustBase); err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	return nil
}

func signTrustBaseCmd(config *rootGenesisConfig) *cobra.Command {
	signCfg := &signTrustBaseConfig{Base: config.Base, Keys: config.Keys}
	var cmd = &cobra.Command{
		Use:   "sign-trust-base",
		Short: "Signs trust base file and appends the signature to the file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return signTrustBaseRunFunc(signCfg)
		},
	}
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVar(&signCfg.TrustBaseFile, rootTrustBaseFileName, "", "path to root trust base file (default: $AB_HOME/root-trust-base.json)")
	return cmd
}

func signTrustBaseRunFunc(config *signTrustBaseConfig) error {
	// load or generate keys
	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to read root chain keys from file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return fmt.Errorf("failed to extract peer id from key file '%s': %w", config.Keys.GetKeyFileLocation(), err)
	}
	// load trust base file
	trustBase, err := types.NewTrustBaseFromFile(config.getTrustBaseFile())
	if err != nil {
		return fmt.Errorf("failed to load trust base file '%s': %w", config.TrustBaseFile, err)
	}
	// sign trust base file
	if err = trustBase.Sign(peerID.String(), keys.SigningPrivateKey); err != nil {
		return fmt.Errorf("root genesis add signature failed: %w", err)
	}
	// write trust base file
	if err = util.WriteJsonFile(config.getTrustBaseFile(), trustBase); err != nil {
		return fmt.Errorf("root genesis save failed: %w", err)
	}
	return nil
}
