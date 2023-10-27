package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/api/genesis"
	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/client/wallet/money/backend"
	util2 "github.com/alphabill-org/alphabill/common/util"
	money2 "github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/spf13/cobra"
)

const (
	moneyBackendHomeDir = "money-backend"

	serverAddrCmdName       = "server-addr"
	dbFileCmdName           = "db"
	listBillsPageLimit      = "list-bills-page-limit"
	systemIdentifierCmdName = "system-identifier"
)

type moneyBackendConfig struct {
	Base               *baseConfiguration
	AlphabillUrl       string
	ServerAddr         string
	DbFile             string
	LogLevel           string
	LogFile            string
	ListBillsPageLimit int
	InitialBillID      uint64
	InitialBillValue   uint64
	SDRFiles           []string // system description record files
	SystemID           []byte   // hex encoded money system identifier
}

func (c *moneyBackendConfig) GetDbFile() (string, error) {
	dbFile := c.DbFile
	if dbFile == "" {
		dbFile = filepath.Join(c.Base.HomeDir, moneyBackendHomeDir, backend.BoltBillStoreFileName)
	}
	err := os.MkdirAll(filepath.Dir(dbFile), 0700) // -rwx------
	if err != nil {
		return "", fmt.Errorf("failed to create directory for database file: %w", err)
	}
	return dbFile, nil
}

func (c *moneyBackendConfig) getSDRFiles() ([]*genesis.SystemDescriptionRecord, error) {
	var sdrs []*genesis.SystemDescriptionRecord
	if len(c.SDRFiles) == 0 {
		sdrs = append(sdrs, defaultMoneySDR)
	} else {
		for _, sdrFile := range c.SDRFiles {
			sdr, err := util2.ReadJsonFile(sdrFile, &genesis.SystemDescriptionRecord{})
			if err != nil {
				return nil, err
			}
			sdrs = append(sdrs, sdr)
		}
	}
	return sdrs, nil
}

// newMoneyBackendCmd creates a new cobra command for the money-backend component.
func newMoneyBackendCmd(baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyBackendConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "money-backend",
		Short: "Starts money backend service",
		Long:  "Starts money backend service, indexes all transactions by owner predicates, starts http server",
	}
	walletCmd.AddCommand(startMoneyBackendCmd(config))
	return walletCmd
}

func startMoneyBackendCmd(config *moneyBackendConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execMoneyBackendStartCmd(cmd.Context(), config)
		},
	}
	cmd.Flags().StringVarP(&config.AlphabillUrl, alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill node url")
	cmd.Flags().StringVarP(&config.ServerAddr, serverAddrCmdName, "s", defaultAlphabillApiURL, "server address")
	cmd.Flags().StringVarP(&config.DbFile, dbFileCmdName, "f", "", "path to the database file (default: $AB_HOME/"+moneyBackendHomeDir+"/"+backend.BoltBillStoreFileName+")")
	cmd.Flags().IntVarP(&config.ListBillsPageLimit, listBillsPageLimit, "l", 100, "GET /list-bills request default/max limit size")
	cmd.Flags().Uint64Var(&config.InitialBillValue, "initial-bill-value", 100000000, "initial bill value (needed for initial startup only)")
	cmd.Flags().Uint64Var(&config.InitialBillID, "initial-bill-id", 1, "initial bill id hex string with 0x prefix (needed for initial startup only)")
	cmd.Flags().StringSliceVarP(&config.SDRFiles, "system-description-record-files", "c", nil, "path to SDR files (one for each partition, including money partition itself; defaults to single money partition only SDR; needed for initial startup only)")
	cmd.Flags().BytesHexVar(&config.SystemID, systemIdentifierCmdName, money2.DefaultSystemIdentifier, "system identifier in hex format")
	return cmd
}

func execMoneyBackendStartCmd(ctx context.Context, config *moneyBackendConfig) error {
	dbFile, err := config.GetDbFile()
	if err != nil {
		return err
	}
	sdrFiles, err := config.getSDRFiles()
	if err != nil {
		return err
	}
	return backend.Run(ctx, &backend.Config{
		ABMoneySystemIdentifier: config.SystemID,
		AlphabillUrl:            config.AlphabillUrl,
		ServerAddr:              config.ServerAddr,
		DbFile:                  dbFile,
		ListBillsPageLimit:      config.ListBillsPageLimit,
		InitialBill: backend.InitialBill{
			Id:        money2.NewBillID(nil, util2.Uint64ToBytes(config.InitialBillID)),
			Value:     config.InitialBillValue,
			Predicate: templates.AlwaysTrueBytes(),
		},
		SystemDescriptionRecords: sdrFiles,
		Logger:                   config.Base.Logger,
	})
}
