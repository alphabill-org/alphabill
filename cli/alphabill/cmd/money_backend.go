package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	indexer "github.com/alphabill-org/alphabill/pkg/wallet/backend/money"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/spf13/cobra"
)

const (
	moneyBackendHomeDir = "money-backend"
)

type moneyBackendConfig struct {
	Base               *baseConfiguration
	AlphabillUrl       string
	ServerAddr         string
	DbFile             string
	LogLevel           string
	LogFile            string
	ListBillsPageLimit int
}

func (c *moneyBackendConfig) GetDbFile() (string, error) {
	if c.DbFile != "" {
		return c.DbFile, nil
	}
	indexerHomeDir := filepath.Join(c.Base.HomeDir, moneyBackendHomeDir)
	err := os.MkdirAll(indexerHomeDir, 0700) // -rwx------
	if err != nil {
		return "", err
	}
	return filepath.Join(indexerHomeDir, indexer.BoltBillStoreFileName), nil
}

// newMoneyBackendCmd creates a new cobra command for the money-backend component.
func newMoneyBackendCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &moneyBackendConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "money-backend",
		Short: "starts money backend service",
		Long:  "starts money backend service, indexes all transactions by owner predicates, starts http server",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// initialize config so that baseConfig.HomeDir gets configured
			err := initializeConfig(cmd, baseConfig)
			if err != nil {
				return err
			}
			// init logger
			return initWalletLogger(&walletConfig{LogLevel: config.LogLevel, LogFile: config.LogFile})
		},
		Run: func(cmd *cobra.Command, args []string) {
			consoleWriter.Println("Error: must specify a subcommand")
		},
	}
	walletCmd.PersistentFlags().StringVar(&config.LogFile, logFileCmdName, "", fmt.Sprintf("log file path (default output to stderr)"))
	walletCmd.PersistentFlags().StringVar(&config.LogLevel, logLevelCmdName, "INFO", fmt.Sprintf("logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)"))
	walletCmd.AddCommand(startMoneyBackendCmd(ctx, config))
	return walletCmd
}

func startMoneyBackendCmd(ctx context.Context, config *moneyBackendConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execMoneyBackendStartCmd(ctx, cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.AlphabillUrl, alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill node url")
	cmd.Flags().StringVarP(&config.ServerAddr, serverAddrCmdName, "s", "localhost:9654", "server address")
	cmd.Flags().StringVarP(&config.DbFile, dbFileCmdName, "f", "", "path to the database file (default: $AB_HOME/"+moneyBackendHomeDir+"/"+indexer.BoltBillStoreFileName+")")
	cmd.Flags().IntVarP(&config.ListBillsPageLimit, listBillsPageLimit, "l", 100, "GET /list-bills request default/max limit size")
	return cmd
}

func execMoneyBackendStartCmd(ctx context.Context, _ *cobra.Command, config *moneyBackendConfig) error {
	abclient := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})
	dbFile, err := config.GetDbFile()
	if err != nil {
		return err
	}
	store, err := indexer.NewBoltBillStore(dbFile)
	if err != nil {
		return err
	}
	bp := indexer.NewBlockProcessor(store, backend.NewTxConverter(defaultABMoneySystemIdentifier))
	w := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()

	service := indexer.New(w, store)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		service.StartProcess(ctx)
		wg.Done()
	}()

	server := indexer.NewHttpServer(config.ServerAddr, config.ListBillsPageLimit, service)
	err = server.Start()
	if err != nil {
		service.Shutdown()
		return aberrors.Wrap(err, "error starting wallet backend http server")
	}

	// listen for termination signal and shutdown the app
	hook := func(sig os.Signal) {
		wlog.Info("Received signal '", sig, "' shutting down application...")
		err := server.Shutdown(context.Background())
		if err != nil {
			wlog.Error("error shutting down server: ", err)
		}
		service.Shutdown()
	}
	listen(hook, syscall.SIGTERM, syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGINT)

	wg.Wait() // wait for service shutdown to complete

	return nil
}
