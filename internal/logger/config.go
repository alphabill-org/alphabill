package logger

import (
	"io"
	"os"
)

type GlobalConfig struct {
	DefaultLevel    LogLevel
	PackageLevels   map[string]LogLevel
	Writer          io.Writer // Writer for log output. By default os.Stdout
	ConsoleFormat   bool      // set to true for human-readable output. By default, uses JSON output.
	ShowCaller      bool      // set to true to show caller in the log message. By default, true.
	TimeLocation    string    // the location for displaying time, by default OS local time.
	ShowGoroutineID bool      // set to true to add goroutine id to the messages. By default, false.
	ShowNodeID      bool      // set to true to add node id to the messages. By default, false.
}

const (
	TimeLocationLocal   = "Local"
	defaultTimeLocation = TimeLocationLocal
)

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
		ShowNodeID:      true,
	}
	return conf
}

// BenchmarkConfiguration is a quiet configuration meant for benchmarks.
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
	return conf
}
