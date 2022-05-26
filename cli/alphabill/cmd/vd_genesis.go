package cmd

import (
	"context"
	"os"
	"path"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/util"

	"github.com/libp2p/go-libp2p-core/peer"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/verifiable_data"

	"github.com/spf13/cobra"
)

const (
	vdGenesisFileName = "node-genesis.json"
	vdDir             = "vd"
)

var defaultVDSystemIdentifier = []byte{0, 0, 0, 1}

type vdGenesisConfig struct {
	Base             *baseConfiguration
	Keys             *KeysConfig
	SystemIdentifier []byte
	Output           string
}

// newVDGenesisCmd creates a new cobra command for the vd genesis.
func newVDGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &vdGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig)}
	var cmd = &cobra.Command{
		Use:   "vd-genesis",
		Short: "Generates a genesis file for the Verifiable Data partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return vdGenesisRunFun(ctx, config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", defaultVDSystemIdentifier, "system identifier in HEX format")
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/vd/node-genesis.json)")
	config.Keys.addCmdFlags(cmd, vdDir)
	return cmd
}

func vdGenesisRunFun(_ context.Context, config *vdGenesisConfig) error {
	vdHomePath := path.Join(config.Base.HomeDir, vdDir)
	if !util.FileExists(vdHomePath) {
		err := os.MkdirAll(vdHomePath, 0700) // -rwe------)
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(vdHomePath)
	if util.FileExists(nodeGenesisFile) {
		return errors.Errorf("node genesis %s exists", nodeGenesisFile)
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", config.Keys.GetKeyFileLocation())
	}

	txSystem, err := verifiable_data.New()
	if err != nil {
		return err
	}

	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		txSystem,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
	)
	if err != nil {
		return err
	}
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *vdGenesisConfig) getNodeGenesisFileLocation(vdHomePath string) string {
	if c.Output != "" {
		return c.Output
	}
	return path.Join(vdHomePath, vdGenesisFileName)
}
