package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/pkg/client"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

type walletBackendConfig struct {
	Base         *baseConfiguration
	AlphabillUrl string
	Pubkeys      []string
	LogLevel     string
	LogFile      string
}

func (c *walletBackendConfig) GetPubKeys() ([][]byte, error) {
	pubkeys := make([][]byte, len(c.Pubkeys))
	for i, pubKey := range c.Pubkeys {
		pubKeyBytes, err := hexutil.Decode(pubKey)
		if err != nil {
			return nil, err
		}
		if len(pubKeyBytes) != 33 {
			return nil, errors.New("invalid pubkey format")
		}
		pubkeys[i] = pubKeyBytes
	}
	return pubkeys, nil
}

// newWalletBackendCmd creates a new cobra command for the wallet-backend component.
func newWalletBackendCmd(_ context.Context, baseConfig *baseConfiguration) *cobra.Command {
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
	walletCmd.PersistentFlags().StringVar(&config.LogFile, logFileCmdName, "", fmt.Sprintf("log file path (default output to stderr)"))
	walletCmd.PersistentFlags().StringVar(&config.LogLevel, logLevelCmdName, "INFO", fmt.Sprintf("logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)"))
	walletCmd.AddCommand(startCmd(config))
	return walletCmd
}

func startCmd(config *walletBackendConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execStartCmd(cmd, config)
		},
	}
	cmd.Flags().StringVarP(&config.AlphabillUrl, alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill uri to connect to")
	cmd.Flags().StringSliceVarP(&config.Pubkeys, "pubkeys", "p", nil, "pubkeys to index")
	return cmd
}

func execStartCmd(_ *cobra.Command, config *walletBackendConfig) error {
	abclient := client.New(client.AlphabillClientConfig{Uri: config.AlphabillUrl})
	pubkeys, err := config.GetPubKeys()
	if err != nil {
		return err
	}

	service := backend.New(pubkeys, abclient, backend.NewInmemoryBillStore())
	wg := service.StartProcess(context.Background())

	server := backend.NewHttpServer(":8082", service)
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
