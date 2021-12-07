package cmd

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
)

type (
	rootConfiguration struct {
		// The Alphabill home directory
		HomeDir string
		// Configuration file URL. If it's relative, then it's relative from the HomeDir.
		CfgFile string
	}
)

const (
	// The prefix for configuration keys inside environment.
	envPrefix = "AB"
	// The default name for config file.
	defaultConfigFile = "config.props"
	// The default home directory.
	defaultHomeDir = "$HOME/.alphabill"
)

func (r *rootConfiguration) addRootConfigurationFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVar(&r.HomeDir, "home", defaultHomeDir, "set the AB_HOME for this invocation (default is $HOME/.alphabill")
	cmd.PersistentFlags().StringVar(&r.CfgFile, "config", "", "config file location (default is $AB_HOME/config.yaml)")
}

func (r *rootConfiguration) initConfigFileLocation() {
	if r.CfgFile == "" {
		r.CfgFile = defaultConfigFile
	}
	if !strings.HasPrefix(r.CfgFile, string(os.PathSeparator)) {
		// Relative path
		r.CfgFile = r.HomeDir + string(os.PathSeparator) + r.CfgFile
	}
}

func (r *rootConfiguration) configFileExists() bool {
	_, err := os.Stat(r.CfgFile)
	return err == nil
}
