package logger

import (
	"bufio"
	"os"
	"regexp"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
)

func init() {
	initializeGlobalFactory()
	initializeGlobalConfig()
}

func initializeGlobalFactory() {
	globalFactoryImpl = &globalFactory{
		loggers:                 make(map[string]*ContextLogger),
		context:                 make(Context),
		consoleTimeFormat:       "15:04:05.000",
		callerSkipFrames:        4, // This depends on the logger code, not meant to be changed by callers.
		packageNameResolver:     &PackageNameResolver{BasePackage: "kcash-proto"},
		nonAlphaNumericRegex:    regexp.MustCompile(`[^a-zA-Z0-9]`),
		globalLoggerInitialized: false,
	}

}

func initializeGlobalConfig() {
	type (
		LoggerConfiguration struct {
			DefaultLevel    string            `yaml:"defaultLevel"` // tags enable to parse yaml file for the configuration in the future
			PackageLevels   map[string]string `yaml:"packageLevels"`
			OutputPath      string            `yaml:"outputPath"`
			ConsoleFormat   bool              `yaml:"consoleFormat"`
			ShowCaller      bool              `yaml:"showCaller"`
			TimeLocation    string            `yaml:"timeLocation"`
			ShowGoroutineID bool              `yaml:"showGoroutineID"`
		}
	)

	config := &LoggerConfiguration{}

	// TODO read a config file.

	// --- Setup globals
	globalConfig := GlobalConfig{
		DefaultLevel:    LevelFromString(config.DefaultLevel),
		PackageLevels:   make(map[string]LogLevel),
		Writer:          nil,
		ConsoleFormat:   config.ConsoleFormat,
		ShowCaller:      config.ShowCaller,
		TimeLocation:    config.TimeLocation,
		ShowGoroutineID: config.ShowGoroutineID,
	}
	// Output writer
	if config.OutputPath != "" {
		file, err := os.Create(config.OutputPath)
		if err != nil {
			panic(errors.Wrap(err, "failed to create output writer"))
		}
		globalConfig.Writer = bufio.NewWriter(file)
	} else {
		globalConfig.Writer = os.Stdout
	}
	// Log levels for individual packages
	for k, v := range config.PackageLevels {
		globalConfig.PackageLevels[k] = LevelFromString(v)
	}

	UpdateGlobalConfig(globalConfig)
}
