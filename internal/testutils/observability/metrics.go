package observability

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	tnop "go.opentelemetry.io/otel/trace/noop"

	testlogr "github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/logger"
)

/*
NOP creates observability implementation where everything is no-op.
Use it for tests for which it absolutely doesn't make sense to create any logs, traces or metrics.
*/
func NOPObservability() *Observability {
	return &Observability{
		mp:   noop.NewMeterProvider(),
		tp:   tnop.NewTracerProvider(),
		logF: func(lc *logger.LogConfiguration) (*slog.Logger, error) { return testlogr.NOP(), nil },
	}
}

/*
Default creates observability implementation based on environment variables.
*/
func Default(t *testing.T) *Observability {
	return New(t, "", "", testlogr.LoggerBuilder(t))
}

func New(t *testing.T, metrics, traces string, logBuilder func(*logger.LogConfiguration) (*slog.Logger, error)) *Observability {
	initPropagator()

	obs := &Observability{
		mp:   noop.NewMeterProvider(),
		tp:   tnop.NewTracerProvider(),
		logF: logBuilder,
	}

	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName("alphabill"),
		attribute.String("test.name", t.Name()),
		attribute.Int64("test.id", testID(t)),
	)

	if traceExp := env("AB_TEST_TRACER", traces); traceExp != "" {
		tp, err := newTraceProvider(traceExp, res)
		if err != nil {
			t.Fatal("failed to init trace exporter", err)
		}
		obs.tp = tp
		t.Cleanup(func() {
			if err := tp.Shutdown(context.Background()); err != nil {
				t.Logf("shutting down trace exporter: %v", err)
			}
		})
	}

	return obs
}

type Observability struct {
	logF func(*logger.LogConfiguration) (*slog.Logger, error)
	tp   trace.TracerProvider
	mp   metric.MeterProvider
}

func (o *Observability) Logger() *slog.Logger {
	log, err := o.logF(nil)
	if err != nil {
		panic(fmt.Errorf("unexpectedly log builder returned error: %w", err))
	}
	return log
}

func (o *Observability) RoundLogger(curRound func() uint64) *slog.Logger {
	log, err := o.logF(nil)
	if err != nil {
		panic(fmt.Errorf("unexpectedly log builder returned error: %w", err))
	}
	return slog.New(logger.NewRoundHandler(log.Handler(), curRound))
}

func (o *Observability) Factory() Factory {
	return Factory{
		logF: o.logF,
		obsF: func(metrics, traces string) (*Observability, error) { return o, nil },
	}
}

func (o *Observability) Meter(name string, options ...metric.MeterOption) metric.Meter {
	return o.mp.Meter(name, options...)
}

func (o *Observability) MetricsHandler() http.Handler {
	return nil
}

func (o *Observability) PrometheusRegisterer() prometheus.Registerer {
	return nil
}

func (o *Observability) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return o.tp.Tracer(name, options...)
}

func (o *Observability) TracerProvider() trace.TracerProvider { return o.tp }

func (o *Observability) Shutdown() error { return nil }

func newTraceProvider(exporter string, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	var err error
	var exp sdktrace.SpanExporter

	switch exporter {
	case "stdout":
		exp, err = stdouttrace.New()
	case "otlptracehttp":
		exp, err = otlptracehttp.New(context.Background(), otlptracehttp.WithInsecure())
	case "zipkin":
		exp, err = zipkin.New("")
	default:
		return nil, fmt.Errorf("unsupported trace exporter %q", exporter)
	}
	if err != nil {
		return nil, fmt.Errorf("creating %q exporter: %w", exporter, err)
	}

	return sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exp),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	), nil
}

/*
Global registry for test IDs.
In tests sometimes ad-hoc client is used which do not propagate trace-id
so attach an ID to traces which should allow to filter out all traces for
particular test run.
*/
var (
	reg map[*testing.T]int64
	m   sync.Mutex
)

func testID(t *testing.T) int64 {
	m.Lock()
	defer m.Unlock()

	if reg == nil {
		reg = make(map[*testing.T]int64)
	}
	if id, ok := reg[t]; ok {
		return id
	}
	id := rand.Int63() // #nosec G404
	reg[t] = id
	t.Cleanup(func() {
		m.Lock()
		delete(reg, t)
		m.Unlock()
	})

	return id
}

func env(name, defaultValue string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return defaultValue
}

var initPropagator = sync.OnceFunc(func() {
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		))
})
