package observability

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
	testlogr "github.com/alphabill-org/alphabill/testutils/logger"
)

/*
NOP factory can be shared between tests.
*/
var nopFactory = Factory{
	logF: func(lc *logger.LogConfiguration) (*slog.Logger, error) { return testlogr.NOP(), nil },
	obsF: func(metrics, traces string) (*Observability, error) { return NOPMetrics(), nil },
}

/*
NOPFactory returns observability factory implementation where everything returned is no-op.
*/
func NOPFactory(t *testing.T) Factory {
	return nopFactory
}

/*
NewFactory returns observability implementation fot test "t" based on
(global) configuration (ie environment variables).
*/
func NewFactory(t *testing.T) Factory {
	return Factory{
		logF: testlogr.LoggerBuilder(t),
		obsF: defaultObservabilityBuilder(t),
	}
}

type Factory struct {
	logF func(*logger.LogConfiguration) (*slog.Logger, error)
	obsF func(metrics, traces string) (*Observability, error)
}

func (f Factory) Logger(cfg *logger.LogConfiguration) (*slog.Logger, error) {
	return f.logF(cfg)
}

func (f Factory) Observability(metrics, traces string) (observability.MeterAndTracer, error) {
	return f.obsF(metrics, traces)
}

/*
DefaultObserver is a helper to get metrics and tracer out of factory without
needing to provide parameters (ie defaults are used) and handle error return
value (panics in case of error).
*/
func (f Factory) DefaultObserver() observability.MeterAndTracer {
	obs, err := f.obsF("", "")
	if err != nil {
		panic(fmt.Errorf("building default observability: %w", err))
	}
	return obs
}

/*
DefaultLogger is a helper to get logger out of factory without
needing to provide parameters (ie defaults are used) and handle error return
value (panics in case of error).
*/
func (f Factory) DefaultLogger() *slog.Logger {
	log, err := f.logF(nil)
	if err != nil {
		panic(fmt.Errorf("building default logger: %w", err))
	}
	return log
}

func defaultObservabilityBuilder(t *testing.T) func(metrics, traces string) (*Observability, error) {
	obs := Default(t)
	return func(metrics, traces string) (*Observability, error) {
		return obs, nil
	}
}
