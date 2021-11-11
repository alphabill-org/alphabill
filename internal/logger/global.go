package logger

import (
	"regexp"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type globalFactory struct {
	sync.Mutex
	config                  GlobalConfig
	loggers                 map[string]*ContextLogger
	context                 Context
	consoleTimeFormat       string
	callerSkipFrames        int // how many frames to skip to get real caller. Not meant to be changed by callers.
	packageNameResolver     *PackageNameResolver
	nonAlphaNumericRegex    *regexp.Regexp
	globalLoggerInitialized bool
}

// Singleton for managing application wide logging.
var globalFactoryImpl *globalFactory

// Sets context for all loggers
func SetContext(key string, value interface{}) {
	globalFactoryImpl.setContext(key, value)
}

// Will clear a context key from all loggers
func ClearContext(key string) {
	globalFactoryImpl.clearContext(key)
}

// Will clear all context keys
func ClearAllContext() {
	globalFactoryImpl.clearAllContext()
}

// Creates logger named after the caller package.
func CreateForPackage() Logger {
	return Create(globalFactoryImpl.packageNameResolver.PackageName())
}

// Creates custom named logger
func Create(name string) Logger {
	return globalFactoryImpl.create(name)
}

// Updates global config and updates all loggers accordingly
// Sets only fields that are non-nil
func UpdateGlobalConfig(config GlobalConfig) {
	globalFactoryImpl.Lock()
	defer globalFactoryImpl.Unlock()

	globalFactoryImpl.updateFromConfig(config)
}

// Sets context for all loggers
func (gf *globalFactory) setContext(key string, value interface{}) {
	gf.Lock()
	defer gf.Unlock()

	gf.context[key] = value
	gf.updateAllLoggers()
}

// Will clear a context key from all loggers
func (gf *globalFactory) clearContext(key string) {
	gf.Lock()
	defer gf.Unlock()

	delete(gf.context, key)
	gf.updateAllLoggers()
}

// Will clear all context keys
func (gf *globalFactory) clearAllContext() {
	gf.Lock()
	defer gf.Unlock()

	gf.context = make(Context)
	gf.updateAllLoggers()
}

func (gf *globalFactory) updateFromConfig(config GlobalConfig) {
	newWriter := config.Writer != nil && config.Writer != gf.config.Writer

	// Update output format only if format related changes occurred
	updateOutputFormat := newWriter ||
		gf.config.ConsoleFormat != config.ConsoleFormat ||
		gf.config.ShowCaller != config.ShowCaller

	if newWriter {
		gf.config.Writer = config.Writer
	}
	gf.config.DefaultLevel = config.DefaultLevel
	gf.config.PackageLevels = config.PackageLevels
	gf.config.ConsoleFormat = config.ConsoleFormat
	gf.config.ShowCaller = config.ShowCaller
	gf.config.ShowGoroutineID = config.ShowGoroutineID

	if updateOutputFormat {
		gf.updateOutputFormat()
	}
	if config.TimeLocation != "" {
		gf.updateTimeLocation(config.TimeLocation)
	}
	gf.updateAllLoggers()
}

func (gf *globalFactory) updateTimeLocation(location string) {
	loc, err := time.LoadLocation(location)
	if err != nil {
		// Fallback to default
		loc, _ = time.LoadLocation(defaultTimeLocation)
	}
	// Set global timestamp func
	zerolog.TimestampFunc = func() time.Time {
		return time.Now().In(loc)
	}
}

func (gf *globalFactory) updateOutputFormat() {
	if gf.globalLoggerInitialized {
		log.Error().Msg("Global logger is already initialized, discarding changes to global log format.")
	} else {
		zerolog.TimeFieldFormat = time.RFC3339Nano
		var newGlobalLogger zerolog.Logger
		if gf.config.ConsoleFormat {
			newGlobalLogger = log.Logger.Output(zerolog.ConsoleWriter{
				Out:          gf.config.Writer,
				TimeFormat:   gf.consoleTimeFormat,
				FormatCaller: consoleFormatCallerLastTwoDirs,
			})
		} else {
			newGlobalLogger = zerolog.New(gf.config.Writer).With().Timestamp().Logger()
		}
		if gf.config.ShowCaller {
			newGlobalLogger = newGlobalLogger.With().CallerWithSkipFrameCount(gf.callerSkipFrames).Logger()
		}
		log.Logger = newGlobalLogger
		gf.globalLoggerInitialized = true
	}
}

func (gf *globalFactory) updateAllLoggers() {
	for name, logger := range gf.loggers {
		logger.update(gf.loggerLevel(name), gf.context, gf.config.ShowGoroutineID)
	}
}

func (gf *globalFactory) create(name string) Logger {
	gf.Lock()
	defer gf.Unlock()

	if !gf.globalLoggerInitialized {
		globalFactoryImpl.updateFromConfig(LoadGlobalConfig())
	}

	normName := gf.normalizeName(name)
	if logger, ok := gf.loggers[normName]; ok {
		return logger
	}
	// Idea is that application/logging configuration can specify the log levels based on logger names.
	// These are arbitrary names, but it's expected each package will create on named after the package name.
	cl := newContextLogger(gf.loggerLevel(normName), gf.context, gf.config.ShowGoroutineID)
	gf.loggers[normName] = cl
	return cl
}

func (gf *globalFactory) normalizeName(name string) string {
	return gf.nonAlphaNumericRegex.ReplaceAllString(name, "_")
}

func (gf *globalFactory) loggerLevel(loggerName string) LogLevel {
	if level, ok := gf.config.PackageLevels[loggerName]; ok {
		return level
	}
	return gf.config.DefaultLevel
}
