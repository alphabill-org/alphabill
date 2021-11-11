package logger

import (
	"io"
	"os"
)

type GlobalConfig struct {
	DefaultLevel    LogLevel
	PackageLevels   map[string]LogLevel
	Writer          io.Writer // Writer for log output. By default os.Stdout
	ConsoleFormat   bool      // set to true for human readable output. By default uses JSON output.
	ShowCaller      bool      // set to true to show caller in the log message. By default true.
	TimeLocation    string    // the location for displaying time, by default OS local time.
	ShowGoroutineID bool      // set to true to add goroutine id to the messages. By default false.
}

const (
	TimeLocationUTC     = "UTC"
	TimeLocationLocal   = "Local"
	defaultTimeLocation = TimeLocationLocal
)

// Returns global logger configuration
// If logger configuration is not provided, then developer specific configuration is returned.
// Otherwise the provided configuration is used to overload default values.
func LoadGlobalConfig() GlobalConfig {
	// This will change, when logger configuration loading from file or env is implemented.
	return developerConfiguration()
}

// Will be used when config loading from env of files is supported
func defaultConfiguration() GlobalConfig {
	conf := GlobalConfig{
		DefaultLevel:    DEBUG,
		PackageLevels:   make(map[string]LogLevel),
		Writer:          os.Stdout,
		ConsoleFormat:   false,
		ShowCaller:      true,
		TimeLocation:    defaultTimeLocation,
		ShowGoroutineID: false,
	}
	conf.PackageLevels["internal_cli_config_dump"] = NONE
	return conf
}

// Developer tuned configuration.
func developerConfiguration() GlobalConfig {
	conf := GlobalConfig{
		DefaultLevel:    DEBUG,
		PackageLevels:   make(map[string]LogLevel),
		Writer:          os.Stdout,
		ConsoleFormat:   true,
		ShowCaller:      true,
		TimeLocation:    TimeLocationLocal,
		ShowGoroutineID: true,
	}
	conf.PackageLevels["internal_domain"] = WARNING
	conf.PackageLevels["internal_domain_remapper"] = WARNING
	conf.PackageLevels["internal_domain_refserdes"] = WARNING
	conf.PackageLevels["internal_domain_canonicalizer"] = WARNING
	conf.PackageLevels["internal_auth"] = WARNING
	conf.PackageLevels["internal_input_po_consumer"] = WARNING
	return conf
}

// Developer tuned configuration.
func BenchmarkConfiguration() GlobalConfig {
	conf := GlobalConfig{
		DefaultLevel:    ERROR,
		PackageLevels:   make(map[string]LogLevel),
		Writer:          os.Stdout,
		ConsoleFormat:   true,
		ShowCaller:      true,
		TimeLocation:    defaultTimeLocation,
		ShowGoroutineID: true,
	}
	conf.PackageLevels["gw_bench"] = INFO
	return conf
}
