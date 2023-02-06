package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	twb "github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
)

func newTokenWalletBackendCmd(ctx context.Context, baseConfig *baseConfiguration) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "token-backend",
		Short: "starts token wallet backend service",
		Long:  "starts token wallet backend service, indexes all transactions by owner predicates, starts http server",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initializeConfig(cmd, baseConfig); err != nil {
				return fmt.Errorf("failed to init configuration: %w", err)
			}
			return nil
		},
	}
	cmd.PersistentFlags().String(logFileCmdName, "", "log file path (default output to stderr)")
	cmd.PersistentFlags().String(logLevelCmdName, "INFO", "logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)")
	cmd.AddCommand(startTokenWalletBackendCmd(ctx, baseConfig))
	return cmd
}

func startTokenWalletBackendCmd(ctx context.Context, config *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokenWalletBackendStartCmd(ctx, cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillUriCmdName, "u", defaultAlphabillUri, "alphabill node url")
	cmd.Flags().StringP(serverAddrCmdName, "s", "localhost:9735", "server address")
	cmd.Flags().StringP(dbFileCmdName, "f", "", "path to the database file")
	return cmd
}

func execTokenWalletBackendStartCmd(ctx context.Context, cmd *cobra.Command, config *baseConfiguration) error {
	abURL, err := cmd.Flags().GetString(alphabillUriCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", alphabillUriCmdName, err)
	}
	srvAddr, err := cmd.Flags().GetString(serverAddrCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", serverAddrCmdName, err)
	}
	dbFile, err := cmd.Flags().GetString(dbFileCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", dbFileCmdName, err)
	}
	if dbFile == "" {
		dbFile = filepath.Join(config.HomeDir, "token-backend", "tokens.db")
	}

	logFile, err := cmd.Flags().GetString(logFileCmdName)
	if err != nil {
		return fmt.Errorf("failed to read %s flag value: %w", logFileCmdName, err)
	}
	logLevel, err := cmd.Flags().GetString(logLevelCmdName)
	if err != nil {
		return fmt.Errorf("failed to read %s flag value: %w", logLevelCmdName, err)
	}
	logger, err := initLogger(logFile, logLevel)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop()
	}()

	return twb.Run(ctx, twb.NewConfig(srvAddr, abURL, dbFile, logger.Error))
}

func initLogger(fileName, logLevel string) (wlog.Logger, error) {
	logWriter := os.Stderr
	if fileName != "" {
		logFile, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600) // -rw-------
		if err != nil {
			return nil, err
		}
		logWriter = logFile
	}

	logger, err := wlog.New(wlog.Levels[logLevel], logWriter)
	if err != nil {
		return nil, err
	}
	return logger, nil
}
