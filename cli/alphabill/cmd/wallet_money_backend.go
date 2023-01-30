package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"sync"
	"syscall"

	"github.com/alphabill-org/alphabill/internal/crypto"
	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

const (
	walletBackendDir   = "wallet-backend"
	serverAddrCmdName  = "server-addr"
	dbFileCmdName      = "db"
	pubkeysCmdName     = "pubkeys"
	listBillsPageLimit = "list-bills-page-limit"
)

type walletBackendConfig struct {
	Base               *baseConfiguration
	AlphabillUrl       string
	ServerAddr         string
	DbFile             string
	Pubkeys            []string
	LogLevel           string
	LogFile            string
	ListBillsPageLimit int
	TrustBaseFile      string
}

func (c *walletBackendConfig) GetPubKeys() ([][]byte, error) {
	pubkeys := make([][]byte, len(c.Pubkeys))
	for i, pubKey := range c.Pubkeys {
		pubKeyBytes, err := hexutil.Decode(pubKey)
		if err != nil {
			return nil, err
		}
		if len(pubKeyBytes) != crypto.CompressedSecp256K1PublicKeySize {
			return nil, fmt.Errorf("invalid pubkey length for key %s", pubKey)
		}
		pubkeys[i] = pubKeyBytes
	}
	return pubkeys, nil
}

func (c *walletBackendConfig) GetDbFile() (string, error) {
	if c.DbFile != "" {
		return c.DbFile, nil
	}
	walletBackendHomeDir := path.Join(c.Base.HomeDir, walletBackendDir)
	err := os.MkdirAll(walletBackendHomeDir, 0700) // -rwx------
	if err != nil {
		return "", err
	}
	return path.Join(walletBackendHomeDir, backend.BoltBillStoreFileName), nil
}

func (c *walletBackendConfig) GetTrustBase() (map[string]crypto.Verifier, error) {
	trustBase, err := util.ReadJsonFile(c.TrustBaseFile, &TrustBase{})
	if err != nil {
		return nil, err
	}
	err = trustBase.verify()
	if err != nil {
		return nil, err
	}
	verifiers, err := trustBase.toVerifiers()
	if err != nil {
		return nil, err
	}
	return verifiers, nil
}

// newWalletBackendCmd creates a new cobra command for the wallet-backend component.
func newWalletBackendCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	config := &walletBackendConfig{Base: baseConfig}
	var walletCmd = &cobra.Command{
		Use:   "wallet-backend",
		Short: "starts wallet backend service",
		Long:  "starts wallet backend service, indexes bills by given public keys, starts http server",
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
	walletCmd.PersistentFlags().StringVar(&config.LogFile, logFileCmdName, "", "log file path (default output to stderr)")
	walletCmd.PersistentFlags().StringVar(&config.LogLevel, logLevelCmdName, "INFO", "logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)")
	walletCmd.AddCommand(startCmd(ctx, config))
	return walletCmd
}

func startCmd(ctx context.Context, config *walletBackendConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execStartCmd(ctx, cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.AlphabillUrl, alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().StringVarP(&config.ServerAddr, serverAddrCmdName, "s", "localhost:9654", "wallet backend server address")
	cmd.Flags().StringVarP(&config.DbFile, dbFileCmdName, "f", "", "path to the database file (default: $AB_HOME/wallet-backend/"+backend.BoltBillStoreFileName+")")
	cmd.Flags().StringSliceVarP(&config.Pubkeys, pubkeysCmdName, "p", nil, "pubkeys to index (more keys can be added to running service through web api)")
	cmd.Flags().IntVarP(&config.ListBillsPageLimit, listBillsPageLimit, "l", 100, "GET /list-bills request default/max limit size")
	cmd.Flags().StringVarP(&config.TrustBaseFile, trustBaseFileCmdName, "t", "", "path to trust base file")
	_ = cmd.MarkFlagRequired(trustBaseFileCmdName)
	return cmd
}

func execStartCmd(ctx context.Context, _ *cobra.Command, config *walletBackendConfig) error {
	abclient := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})
	pubkeys, err := config.GetPubKeys()
	if err != nil {
		return err
	}
	dbFile, err := config.GetDbFile()
	if err != nil {
		return err
	}
	verifiers, err := config.GetTrustBase()
	if err != nil {
		return err
	}
	store, err := backend.NewBoltBillStore(dbFile)
	if err != nil {
		return err
	}
	for _, pubkey := range pubkeys {
		k := backend.NewPubkey(pubkey)
		err = store.AddKey(k)
		if err != nil {
			return err
		}
	}
	// TODO: hardcoded AlphaBill Money SystemId for now, should come from config in the future
	bp := backend.NewBlockProcessor([]byte{0, 0, 0, 0}, store)
	w := wallet.New().SetBlockProcessor(bp).SetABClient(abclient).Build()

	service := backend.New(w, store, verifiers)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		service.StartProcess(ctx)
		wg.Done()
	}()

	server := backend.NewHttpServer(config.ServerAddr, config.ListBillsPageLimit, service)
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

// listen waits for given OS signals and then calls given shutdownHook func
func listen(shutdownHook func(sig os.Signal), signals ...os.Signal) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	sig := <-ch
	shutdownHook(sig)
}
