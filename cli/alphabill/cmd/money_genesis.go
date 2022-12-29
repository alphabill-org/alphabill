package cmd

import (
	"context"
	"crypto"
	"os"
	"path"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/partition"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	moneyPartitionDir         = "money"
	defaultInitialBillId      = 1
	defaultInitialBillValue   = 1000000
	defaultDCMoneySupplyValue = 1000000
	defaultT2Timeout          = 2500
)

var defaultABMoneySystemIdentifier = []byte{0, 0, 0, 0}
var defaultFeeCreditBill = &money.FeeCreditBill{
	SystemID: defaultABMoneySystemIdentifier,
	ID:       uint256.NewInt(2),
	Owner:    script.PredicateAlwaysTrue(),
}

type moneyGenesisConfig struct {
	Base               *baseConfiguration
	SystemIdentifier   []byte
	Keys               *keysConfig
	Output             string
	InitialBillValue   uint64 `validate:"gte=0"`
	DCMoneySupplyValue uint64 `validate:"gte=0"`
	T2Timeout          uint32 `validate:"gte=0"`
	FeeCreditBillFiles []string
}

type feeCreditBill struct {
	SystemID    string `json:"systemId"`
	UnitID      string `json:"unitId"`
	OwnerPubKey string `json:"ownerPubKey"`
}

// newMoneyGenesisCmd creates a new cobra command for the alphabill money partition genesis.
func newMoneyGenesisCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyGenesisConfig{Base: baseConfig, Keys: NewKeysConf(baseConfig, moneyPartitionDir)}
	var cmd = &cobra.Command{
		Use:   "money-genesis",
		Short: "Generates a genesis file for the Alphabill Money partition",
		RunE: func(cmd *cobra.Command, args []string) error {
			return abMoneyGenesisRunFun(ctx, config)
		},
	}

	cmd.Flags().BytesHexVarP(&config.SystemIdentifier, "system-identifier", "s", defaultABMoneySystemIdentifier, "system identifier in HEX format")
	config.Keys.addCmdFlags(cmd)
	cmd.Flags().StringVarP(&config.Output, "output", "o", "", "path to the output genesis file (default: $AB_HOME/money/node-genesis.json)")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", defaultInitialBillValue, "the initial bill value")
	cmd.Flags().Uint64Var(&config.DCMoneySupplyValue, "dc-money-supply-value", defaultDCMoneySupplyValue, "the initial value for Dust Collector money supply. Total money sum is initial bill + DC money supply.")
	cmd.Flags().Uint32Var(&config.T2Timeout, "t2-timeout", defaultT2Timeout, "time interval for how long root chain waits before re-issuing unicity certificate, in milliseconds")
	cmd.Flags().StringSliceVarP(&config.FeeCreditBillFiles, "fee-credit-files", "c", nil, "path to fee credit bill files (one for each partition, including money partion itself; defaults to single money partition only fee credit bill [id=2 owner=always-true-predicate])")
	return cmd
}

func abMoneyGenesisRunFun(_ context.Context, config *moneyGenesisConfig) error {
	moneyPartitionHomePath := path.Join(config.Base.HomeDir, moneyPartitionDir)
	if !util.FileExists(moneyPartitionHomePath) {
		err := os.MkdirAll(moneyPartitionHomePath, 0700) // -rwx------
		if err != nil {
			return err
		}
	}

	nodeGenesisFile := config.getNodeGenesisFileLocation(moneyPartitionHomePath)
	if util.FileExists(nodeGenesisFile) {
		return errors.Errorf("node genesis %s exists", nodeGenesisFile)
	} else if err := os.MkdirAll(filepath.Dir(nodeGenesisFile), 0700); err != nil {
		return err
	}

	keys, err := LoadKeys(config.Keys.GetKeyFileLocation(), config.Keys.GenerateKeys, config.Keys.ForceGeneration)
	if err != nil {
		return errors.Wrapf(err, "failed to load keys %v", config.Keys.GetKeyFileLocation())
	}
	peerID, err := peer.IDFromPublicKey(keys.EncryptionPrivateKey.GetPublic())
	if err != nil {
		return err
	}
	encryptionPublicKeyBytes, err := keys.EncryptionPrivateKey.GetPublic().Raw()
	if err != nil {
		return err
	}

	ib := &money.InitialBill{
		ID:    uint256.NewInt(defaultInitialBillId),
		Value: config.InitialBillValue,
		Owner: script.PredicateAlwaysTrue(),
	}

	fcBills, err := config.getFeeCreditBills()
	if err != nil {
		return err
	}
	txSystem, err := money.NewMoneyTxSystem(
		crypto.SHA256,
		ib,
		fcBills,
		config.DCMoneySupplyValue,
		money.SchemeOpts.SystemIdentifier(config.SystemIdentifier),
	)
	params, err := config.getPartitionParams()
	if err != nil {
		return err
	}
	nodeGenesis, err := partition.NewNodeGenesis(
		txSystem,
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
	return util.WriteJsonFile(nodeGenesisFile, nodeGenesis)
}

func (c *moneyGenesisConfig) getNodeGenesisFileLocation(home string) string {
	if c.Output != "" {
		return c.Output
	}
	return path.Join(home, vdGenesisFileName)
}

func (c *moneyGenesisConfig) getPartitionParams() (*anypb.Any, error) {
	dst := new(anypb.Any)
	fcBills, err := c.getGenesisFeeCreditBills()
	if err != nil {
		return nil, err
	}
	src := &genesis.MoneyPartitionParams{
		InitialBillValue:   c.InitialBillValue,
		DcMoneySupplyValue: c.DCMoneySupplyValue,
		FeeCreditBills:     fcBills,
	}
	err = dst.MarshalFrom(src)
	if err != nil {
		return nil, err
	}
	return dst, nil
}

func (c *moneyGenesisConfig) getFeeCreditBills() ([]*money.FeeCreditBill, error) {
	var fcBills []*money.FeeCreditBill
	if len(c.FeeCreditBillFiles) == 0 {
		fcBills = append(fcBills, defaultFeeCreditBill)
	} else {
		for _, feeBillPath := range c.FeeCreditBillFiles {
			fc, err := util.ReadJsonFile(feeBillPath, &feeCreditBill{})
			if err != nil {
				return nil, err
			}
			fcb, err := fc.toMoneyFeeBill()
			if err != nil {
				return nil, err
			}
			fcBills = append(fcBills, fcb)
		}
	}
	return fcBills, nil
}

func (c *moneyGenesisConfig) getGenesisFeeCreditBills() ([]*genesis.FeeCreditBill, error) {
	fcBills, err := c.getFeeCreditBills()
	if err != nil {
		return nil, err
	}
	var genesisFCBills []*genesis.FeeCreditBill
	for _, fcBill := range fcBills {
		genesisFCBills = append(genesisFCBills, fcBill.ToGenesis())
	}
	return genesisFCBills, nil
}

func (f *feeCreditBill) toMoneyFeeBill() (*money.FeeCreditBill, error) {
	systemID, err := hexutil.Decode(f.SystemID)
	if err != nil {
		return nil, err
	}
	unitID, err := hexutil.Decode(f.UnitID)
	if err != nil {
		return nil, err
	}
	ownerPubKey, err := hexutil.Decode(f.OwnerPubKey)
	if err != nil {
		return nil, err
	}
	return money.NewFeeCreditBill(systemID, unitID, ownerPubKey)
}
