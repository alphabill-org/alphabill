package cli

import (
	"flag"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/flags"
)

const (
	configurationFilePathsFlagName       = "config_path"
	dumpStackTracesOnExitTimeoutFlagName = "dump_stack_traces_on_exit_timeout"
	gracefulExitTimeoutFlagName          = "graceful_exit_timeout"
)

func init() {
	flag.Var(FlagConfigurationFilePaths,
		configurationFilePathsFlagName,
		"Specifies paths for configuration files from where to read component configuration. "+
			"Can pass the same argument multiple times to provide multiple files for configuration.")
}

var (
	FlagConfigurationFilePaths = &flags.StringSliceFlags{}

	// FlagDumpStackTracesOnExitTimeout whether to print out all go routine stack traces when graceful exit timeout
	FlagDumpStackTracesOnExitTimeout = flag.Bool(
		dumpStackTracesOnExitTimeoutFlagName, false,
		"Specify whether to dump go routine stack traces when component graceful exit times out. Accepts boolean (true, false)")

	// FlagGracefulExitTimeout (if greater than 0) will overwrite component default exit timeout
	FlagGracefulExitTimeout = flag.Duration(
		gracefulExitTimeoutFlagName, 0,
		"Specify how long to wait between receiving SIGINT/SIGTERM signal and force closing. "+
			"Accepts duration string (examples: 300ms, 5s, 2h45m). "+
			"By default, will use hard-coded value of 6s")
)
