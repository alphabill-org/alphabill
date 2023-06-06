package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/alphabill-org/alphabill/pkg/wallet/tokens/backend"
)

const defaultTokensBackendApiURL = "localhost:9735"

func newTokensBackendCmd(baseConfig *baseConfiguration) *cobra.Command {
	var cmd = &cobra.Command{
		Use:   "tokens-backend",
		Short: "Starts tokens backend service",
		Long:  "Starts tokens backend service, indexes all transactions by owner predicates, starts http server",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := initializeConfig(cmd, baseConfig); err != nil {
				return fmt.Errorf("failed to init configuration: %w", err)
			}
			return nil
		},
	}
	cmd.PersistentFlags().String(logFileCmdName, "", "log file path (default output to stderr)")
	cmd.PersistentFlags().String(logLevelCmdName, "INFO", "logging level (DEBUG, INFO, NOTICE, WARNING, ERROR)")
	cmd.AddCommand(buildCmdStartTokensBackend(baseConfig))
	return cmd
}

func buildCmdStartTokensBackend(config *baseConfiguration) *cobra.Command {
	cmd := &cobra.Command{
		Use: "start",
		RunE: func(cmd *cobra.Command, args []string) error {
			return execTokensBackendStartCmd(cmd.Context(), cmd, config)
		},
	}
	cmd.Flags().StringP(alphabillNodeURLCmdName, "u", defaultAlphabillNodeURL, "alphabill node url")
	cmd.Flags().StringP(serverAddrCmdName, "s", defaultTokensBackendApiURL, "server address")
	cmd.Flags().StringP(dbFileCmdName, "f", "", "path to the database file")
	return cmd
}

func execTokensBackendStartCmd(ctx context.Context, cmd *cobra.Command, config *baseConfiguration) error {
	abURL, err := cmd.Flags().GetString(alphabillNodeURLCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", alphabillNodeURLCmdName, err)
	}
	srvAddr, err := cmd.Flags().GetString(serverAddrCmdName)
	if err != nil {
		return fmt.Errorf("failed to get %q flag value: %w", serverAddrCmdName, err)
	}
	dbFile, err := filenameEnsureDir(dbFileCmdName, cmd, config.HomeDir, "tokens-backend", "tokens.db")
	if err != nil {
		return fmt.Errorf("failed to get path for database: %w", err)
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

	return backend.Run(ctx, backend.NewConfig(srvAddr, abURL, dbFile, logger))
}

/*
filenameEnsureDir assumes that "flagName" is a filename parameter in "cmd" and returns it's value
or name constructed from defaultPathElements (when the flag value is empty). It also ensures that
the directory of the file exists (ie creates it when it doesn't exist already).
*/
func filenameEnsureDir(flagName string, cmd *cobra.Command, defaultPathElements ...string) (string, error) {
	fileName, err := cmd.Flags().GetString(flagName)
	if err != nil {
		return "", fmt.Errorf("failed to get %q flag value: %w", flagName, err)
	}
	if fileName == "" {
		fileName = filepath.Join(defaultPathElements...)
	}

	if err := os.MkdirAll(filepath.Dir(fileName), 0700); err != nil {
		return "", fmt.Errorf("failed to create directory path: %w", err)
	}
	return fileName, nil
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