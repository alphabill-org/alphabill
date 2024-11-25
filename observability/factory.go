package observability

import (
	"fmt"
	"log/slog"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

func NewFactory() Factory {
	return Factory{}
}

type Factory struct{}

func (Factory) Logger(cfg *logger.LogConfiguration) (*slog.Logger, error) {
	logger, err := logger.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating logger: %w", err)
	}
	return logger, nil
}

func (Factory) Observability(metrics, traces string) (MeterAndTracer, error) {
	return newObservability(metrics, traces)
}

// MeterAndTracer combines Metrics and Tracing related methods
type MeterAndTracer interface {
	Tracer(name string, options ...trace.TracerOption) trace.Tracer
	TracerProvider() trace.TracerProvider

	Meter(name string, opts ...metric.MeterOption) metric.Meter
	PrometheusRegisterer() prometheus.Registerer

	Shutdown() error
}

/*
WithLogger combines "metrics and tracing provider" and logger into
single "observability provider".
It is mostly meant to be used to "replace logger" in the situations where
sublogger is created (ie logger.With) for a scope and needs to be passed
as single observability parameter.
*/
func WithLogger(obs MeterAndTracer, log *slog.Logger) MeterTracerLogger {
	if wl, ok := obs.(MeterTracerLogger); ok {
		return MeterTracerLogger{mat: wl.mat, log: log}
	}
	return MeterTracerLogger{mat: obs, log: log}
}

type MeterTracerLogger struct {
	mat MeterAndTracer
	log *slog.Logger
}

func (mtl MeterTracerLogger) Logger() *slog.Logger { return mtl.log }

/*
RoundLogger returns logger which adds Round attribute to each log record, the
curRound callback is used to get the current round value.
*/
func (mtl MeterTracerLogger) RoundLogger(curRound func() uint64) *slog.Logger {
	return slog.New(logger.NewRoundHandler(mtl.log.Handler(), curRound))
}

func (mtl MeterTracerLogger) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return mtl.mat.Tracer(name, options...)
}

func (mtl MeterTracerLogger) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return mtl.mat.Meter(name, opts...)
}

func (mtl MeterTracerLogger) PrometheusRegisterer() prometheus.Registerer {
	return mtl.mat.PrometheusRegisterer()
}

func (mtl MeterTracerLogger) TracerProvider() trace.TracerProvider { return mtl.mat.TracerProvider() }

func (mtl MeterTracerLogger) Shutdown() error { return mtl.mat.Shutdown() }
