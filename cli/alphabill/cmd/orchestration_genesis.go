package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/partition"
	"github.com/alphabill-org/alphabill/state"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
)

const (
	orchestrationGenesisFileName      = "node-genesis.json"
	orchestrationGenesisStateFileName = "node-genesis-state.cbor"
	orchestrationPartitionDir         = "orchestration"
)

type orchestrationGenesisConfig struct {
	Base             *baseConfiguration
	SystemIdentifier types.SystemID
	Keys             *keysConfig
	Output           string
	OutputState      string
	T2Timeout        uint32 `validate:"gte=0"`
	OwnerPredicate   []byte
}

// newOrchestrationGenesisCmd creates a new cobra command for the orchestration partition genesis.
func newOrchestrationGenesisCmd(baseConfig *baseConfiguration) *cobra.Command {
	var systemID uint32
	config := &orchestrationGenesisConfig{
		Base: baseConfig,
		Keys: NewKeysConf(baseConfig, orchestrationPartitionDir),
	}
	var cmd = &cobra.Command{
		Use:   "orchestration-genesis",
		Short: "Generates a genesis file for the Orchestration partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			config.SystemIdentifier = types.SystemID(systemID)
			return orchestrationGenesisRunFun(cmd.Context(), config)
		},
	}

	addSystemIDFlag(cmd, &systemID, orchestration.DefaultSystemID)
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/orchestration/node-genesis.json)")
	cmd.Flags().StringVarP(&config.OutputState, "output-state", "", "", "path to the output genesis state file (default: $AB_HOME/orchestration/node-genesis-state.cbor)")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().BytesHexVar(&config.OwnerPredicate, "owner-predicate", nil, "the Proof-of-Authority owner predicate")
	_ = cmd.MarkFlagRequired("owner-predicate")
	config.Keys.addCmdFlags(cmd)
	return cmd
}

func orchestrationGenesisRunFun(_ context.Context, config *orchestrationGenesisConfig) error {
	homeDir := filepath.Join(config.Base.HomeDir, orchestrationPartitionDir)
	if !util.FileExists(homeDir) {
		err := os.MkdirAll(homeDir, 0700) // -rwx------
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(homeDir)
	if util.FileExists(nodeGenesisFile) {
		return fmt.Errorf("node genesis file %q already exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	nodeGenesisStateFile := config.getNodeGenesisStateFileLocation(homeDir)
	if util.FileExists(nodeGenesisStateFile) {
		return fmt.Errorf("node genesis state file %q already exists", nodeGenesisStateFile)
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return fmt.Errorf("failed to load keys %v: %w", config.Keys.GetKeyFileLocation(), err)
	}

	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}

	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}

	genesisState := state.NewEmptyState()

	params, err := config.getPartitionParams()
	if err != nil {
		return err
	}

	nodeGenesis, err := partition.NewNodeGenesis(
		genesisState,
		partition.WithPeerID(peerID),
		partition.WithSigningKey(keys.SigningPrivateKey),
		partition.WithEncryptionPubKey(encryptionPublicKeyBytes),
		partition.WithSystemIdentifier(config.SystemIdentifier),
		partition.WithT2Timeout(config.T2Timeout),
		partition.WithParams(params),
	)
	if err != nil {
		return err
	}

	if err := writeStateFile(nodeGenesisStateFile, genesisState); err != nil {
		return fmt.Errorf("failed to write genesis state file: %w", err)
	}
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *orchestrationGenesisConfig) getNodeGenesisFileLocation(home string) string {
	if c.Output != "" {
		return c.Output
	}
	return filepath.Join(home, orchestrationGenesisFileName)
}

func (c *orchestrationGenesisConfig) getNodeGenesisStateFileLocation(home string) string {
	if c.OutputState != "" {
		return c.OutputState
	}
	return filepath.Join(home, orchestrationGenesisStateFileName)
}

func (c *orchestrationGenesisConfig) getPartitionParams() ([]byte, error) {
	src := &genesis.OrchestrationPartitionParams{
		OwnerPredicate: c.OwnerPredicate,
	}
	res, err := types.Cbor.Marshal(src)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal orchestration partition params: %w", err)
	}
	return res, nil
}
