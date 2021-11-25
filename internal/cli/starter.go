package cli

import (
	"bytes"
	"context"
	"flag"
	"os"
	"os/signal"
	"reflect"
	"runtime/pprof"
	"strings"
	"syscall"
	"time"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/async"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/lookup"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/cli/viper"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors/errstr"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"

	"github.com/davecgh/go-spew/spew"
)

type (
	ComponentStarter interface {
		Start(ctx context.Context)
	}

	ConfigurationVariableLookuper interface {
		LookupVariable(componentName string, key string) (string, bool)
	}

	CLI struct {
		componentName   string
		starterSupplier func(ctx context.Context) (ComponentStarter, error)
		variables       map[string]*ConfigVariable
		confVarLookuper ConfigurationVariableLookuper

		configPointer     interface{}
		configUnmarshaler *viper.ConfigurationUnmarshaler
	}
)

var (
	log           = logger.CreateForPackage()
	configDumpLog = logger.Create("internal/cli/config_dump")
)

func New(componentName string, configPointer interface{},
	starterSupplier func(ctx context.Context) (ComponentStarter, error),
	opts ...Option,
) (*CLI, error) {
	if starterSupplier == nil {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	if configPointer == nil {
		log.Warning("Configuration struct pointer is nil")
	}
	if configPointer != nil && reflect.ValueOf(configPointer).Kind() != reflect.Ptr {
		return nil, errors.Wrapf(errors.ErrInvalidArgument, "configuration struct argument is not a pointer")
	}

	cliOptions := aggregateCliOptions(componentName, opts)

	return &CLI{
		componentName:   componentName,
		starterSupplier: starterSupplier,
		variables:       make(map[string]*ConfigVariable),
		confVarLookuper: lookup.AlphaBillEnvironmentVariableLookuper,

		configPointer:     configPointer,
		configUnmarshaler: newConfigUnmarshaller(cliOptions.envPrefix),
	}, nil
}

func newConfigUnmarshaller(envPrefix string) *viper.ConfigurationUnmarshaler {
	deserializer := viper.NewBase64StringDeserializer()
	configUnmarshaler := viper.NewConfigurationUnmarshaler(true, true, true,
		envPrefix, deserializer.DecodeHook)

	deserializer.RegisterDeserializer(reflect.TypeOf((*crypto.InMemorySecp256K1Signer)(nil)).Elem(),
		func(bytes []byte) (interface{}, error) {
			return crypto.NewInMemorySecp256K1SignerFromKey(bytes)
		},
	)

	return configUnmarshaler
}

func (c *CLI) ConfigVariableLookup(configurationVariableLookup ConfigurationVariableLookuper) error {
	if c == nil || configurationVariableLookup == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}

	c.confVarLookuper = configurationVariableLookup
	return nil
}

func (c *CLI) AddConfigurationVariables(vars ...*ConfigVariable) error {
	if c == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}

	for _, confVar := range vars {
		err := c.addConfigurationVariable(confVar)
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *CLI) addConfigurationVariable(confVar *ConfigVariable) error {
	if c == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	if confVar == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, "configuration key is nil")
	}

	key := confVar.component + confVar.key

	if _, ok := c.variables[key]; ok {
		return errors.Wrapf(errors.ErrInvalidArgument, "duplicate configuration key: '%s' component: '%s'", confVar.key, confVar.component)
	}

	c.variables[key] = confVar
	return nil
}

// StartAndWait handles wait groups, cancels and signals on its own
// The context passed as an argument has to have a ContextWaitGroup context value defined,
// otherwise StartAndWait will return with an ErrInvalidArgument error
func (c *CLI) StartAndWait(ctx context.Context, opts ...StartupOption) error {
	if c == nil {
		return errors.Wrapf(errors.ErrInvalidArgument, errstr.NilArgument)
	}
	flag.Parse()

	options := aggregateStartupOptions(opts)
	err := c.lookupConfigurationVariables()
	if err != nil {
		return err
	}

	wg, err := async.WaitGroup(ctx)
	if err != nil {
		return err
	}

	ctx, ctxCancel := context.WithCancel(ctx)
	defer ctxCancel()

	log.Info("Creating component %s", c.componentName)
	starter, err := c.starterSupplier(ctx)
	if err != nil {
		return errors.Wrapf(err, "Failed to create an instance of %s", c.componentName)
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigChan)

	wgWaitDone := make(chan bool, 1)
	exitError := make(chan error, 1)
	go func() {
		select {
		case sig := <-sigChan:
			log.Info("Received signal '%s'. Starting graceful shutdown of %s", sig, c.componentName)
			ctxCancel()
		case <-ctx.Done():
			log.Info("Received context done. Starting graceful shutdown of %s", c.componentName)
		case <-wgWaitDone:
			exitError <- nil
			return
		}

		shutdownTimer := time.NewTimer(options.gracefulShutdownTimeout)
		defer shutdownTimer.Stop()
		select {
		case <-wgWaitDone:
			exitError <- nil
		case <-shutdownTimer.C:
			exitError <- c.timeoutError()
		}
	}()

	log.Info("Starting component %s", c.componentName)
	starter.Start(ctx)
	log.Info("Component started: %s", c.componentName)

	go func() {
		wg.Wait()
		wgWaitDone <- true
		log.Info("Finished graceful shutdown of component %s", c.componentName)
	}()

	return <-exitError
}

func (c *CLI) timeoutError() error {
	if FlagDumpStackTracesOnExitTimeout != nil && *FlagDumpStackTracesOnExitTimeout {
		stackTraces, ok := dumpGoroutineStackTraces()
		if ok {
			return errors.Wrapf(errors.ErrTimeout,
				"failed to gracefully shutdown component %s in time. Stacktraces: %s",
				c.componentName, stackTraces)
		}
	}
	return errors.Wrapf(errors.ErrTimeout,
		"failed to gracefully shutdown component %s in time.",
		c.componentName)
}

func (c *CLI) lookupConfigurationVariables() error {
	for _, key := range c.variables {
		componentName := c.componentName
		if key.component != "" {
			componentName = key.component
		}
		keyValue, ok := c.confVarLookuper.LookupVariable(componentName, key.key)
		if ok {
			key.val = &keyValue
		}
	}
	if c.configPointer != nil {
		paths := c.getFilePaths()
		err := c.configUnmarshaler.Unmarshal(c.configPointer, paths...)
		if err != nil {
			return err
		}
		configDumpLog.Debug("Unmarshalled configuration: %s", spew.Sdump(c.configPointer))
	}
	return nil
}

func (c *CLI) getFilePaths() []string {
	configPaths := FlagConfigurationFilePaths
	if configPaths == nil {
		return nil
	}

	var validPaths []string
	for _, path := range *configPaths {
		trimmedPath := strings.TrimSpace(path)
		if len(trimmedPath) > 0 {
			validPaths = append(validPaths, trimmedPath)
		}
	}
	return validPaths
}

func dumpGoroutineStackTraces() (traces string, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
		}
	}()

	buf := bytes.Buffer{}
	profile := pprof.Lookup("goroutine")
	err := profile.WriteTo(&buf, 1)
	if err != nil {
		return "", false
	}
	traces, ok = buf.String(), true
	return
}
