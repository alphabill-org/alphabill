package observability

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/exporters/zipkin"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	tnop "go.opentelemetry.io/otel/trace/noop"
)

func newObservability(metrics, traces string) (*otelMetricsAndTrace, error) {
	res := resource.NewWithAttributes(semconv.SchemaURL,
		semconv.ServiceName(serviceName()),
		semconv.ServiceVersion("0.5.0"),
	)

	o := &otelMetricsAndTrace{mp: noop.NewMeterProvider(), tp: tnop.NewTracerProvider()}

	if metrics != "" {
		mp, err := o.initMeterProvider(metrics, res)
		if err != nil {
			return o, fmt.Errorf("initialize meter provider: %w", err)
		}
		o.mp = mp
		o.shutdownFuncs = append(o.shutdownFuncs, mp.Shutdown)
	}

	if traces != "" {
		tp, err := o.initTraceProvider(traces, res)
		if err != nil {
			return o, fmt.Errorf("initialize trace provider: %w", err)
		}
		o.tp = tp
		o.shutdownFuncs = append(o.shutdownFuncs, tp.Shutdown)
	}

	// set global propagator - http/... middleware use global for default
	// and it is unlikely we want to mock propagator in test
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return o, nil
}

type otelMetricsAndTrace struct {
	tp trace.TracerProvider

	mp metric.MeterProvider
	pr prometheus.Registerer

	shutdownFuncs []func(context.Context) error
}

func (o *otelMetricsAndTrace) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	var errs []error
	for _, fn := range o.shutdownFuncs {
		if err := fn(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("observability shutdown: %w", errors.Join(errs...))
	}
	return nil
}

func (o *otelMetricsAndTrace) TracerProvider() trace.TracerProvider { return o.tp }

func (o *otelMetricsAndTrace) Tracer(name string, options ...trace.TracerOption) trace.Tracer {
	return o.tp.Tracer(name, options...)
}

func (o *otelMetricsAndTrace) Meter(name string, options ...metric.MeterOption) metric.Meter {
	return o.mp.Meter(name, options...)
}

/*
PrometheusRegisterer returns non-nil Prometheus Registerer when "Prometheus pull"
exporter is used, in case of other exporters is returns nil.
*/
func (o *otelMetricsAndTrace) PrometheusRegisterer() prometheus.Registerer {
	return o.pr
}

func (o *otelMetricsAndTrace) initMeterProvider(exporter string, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
	var reader sdkmetric.Reader
	switch exporter {
	case "stdout":
		me, err := stdoutmetric.New()
		if err != nil {
			return nil, fmt.Errorf("creating stdout exporter: %w", err)
		}
		reader = sdkmetric.NewPeriodicReader(me)
	case "prometheus":
		var err error
		o.pr = prometheus.NewRegistry()
		if reader, err = promexp.New(promexp.WithRegisterer(o.pr), promexp.WithNamespace("ab")); err != nil {
			return nil, fmt.Errorf("creating Prometheus exporter: %w", err)
		}
		// the "native" OTEL Go runtime metrics collector seems to not expose all the available
		// metrics so we cheat a bit and use Prometheus' Go runtime collector
		if err := o.pr.Register(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll))); err != nil {
			return nil, fmt.Errorf("registering Go collector with Prometheus registry: %w", err)
		}
		if err := o.pr.Register(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{})); err != nil {
			return nil, fmt.Errorf("registering process collector with Prometheus registry: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported exporter %q", exporter)
	}

	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
	), nil
}

func (o *otelMetricsAndTrace) initTraceProvider(exporter string, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	var err error
	var exp sdktrace.SpanExporter

	switch exporter {
	case "stdout":
		exp, err = stdouttrace.New()
	case "otlptracehttp":
		exp, err = otlptracehttp.New(context.Background())
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
	), nil
}

func serviceName() string {
	for _, v := range os.Args {
		switch v {
		case "wallet", "root", "tokens", "money", "evm", "money-backend", "tokens-backend":
			return "ab." + v
		}
	}

	return "alphabill"
}
