package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	promexp "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
)

func newObservability(metrics string) (*observability, error) {
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName("alphabill"),
			semconv.ServiceVersion("0.3.0"),
		))
	if err != nil {
		return nil, fmt.Errorf("creation OTEL resource: %w", err)
	}

	o := &observability{mp: noop.NewMeterProvider()}

	if metrics != "" {
		mp, err := o.initMeterProvider(metrics, res)
		if err != nil {
			return o, fmt.Errorf("initialize meter provider: %w", err)
		}
		o.mp = mp
		o.shutdownFuncs = append(o.shutdownFuncs, mp.Shutdown)
	}

	// do we need global propagator?
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return o, nil
}

type observability struct {
	mp metric.MeterProvider
	pr prometheus.Registerer

	shutdownFuncs []func(context.Context) error
}

func (o *observability) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func (o *observability) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return o.mp.Meter(name, opts...)
}

func (o *observability) MetricsHandler() http.Handler {
	if o.pr == nil {
		return nil
	}
	return promhttp.HandlerFor(o.pr.(prometheus.Gatherer), promhttp.HandlerOpts{MaxRequestsInFlight: 1})
}

func (o *observability) PrometheusRegisterer() prometheus.Registerer {
	return o.pr
}

func (o *observability) initMeterProvider(exporter string, res *resource.Resource) (*sdkmetric.MeterProvider, error) {
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
	default:
		return nil, fmt.Errorf("unsupported exporter %q", exporter)
	}

	μs := time.Microsecond.Seconds()
	return sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name:  "queued",
					Scope: instrumentation.Scope{Name: "txbuffer"},
				},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						// 1e-9s = 1ns ; 1e-6s = 1μs ; 0.001s = 1ms
						Boundaries: []float64{100e-6, 500e-6, 0.001, 0.01, 0.1, 0.2, 0.4, 0.8, 1.5, 3, 6},
					},
				},
			),
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name:  "exec.tx.time",
					Scope: instrumentation.Scope{Name: "partition.node"},
				},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: []float64{200 * μs, 400 * μs, 800 * μs, 0.0016, 0.003, 0.006, 0.015, 0.03},
					},
				},
			),
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name:  "exec.msg.time",
					Scope: instrumentation.Scope{Name: "partition.node"},
				},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: []float64{100 * μs, 200 * μs, 400 * μs, 800 * μs, 0.0016, 0.01, 0.05},
					},
				},
			),
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name:  "duration",
					Scope: instrumentation.Scope{Name: "grpc_api"},
				},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: []float64{100 * μs, 200 * μs, 400 * μs, 800 * μs, 0.0016, 0.01, 0.05, 0.1},
					},
				},
			),
			sdkmetric.NewView(
				sdkmetric.Instrument{
					Name:  "duration",
					Scope: instrumentation.Scope{Name: "rest_api"},
				},
				sdkmetric.Stream{
					Aggregation: sdkmetric.AggregationExplicitBucketHistogram{
						Boundaries: []float64{100 * μs, 200 * μs, 400 * μs, 800 * μs, 0.0016, 0.01, 0.05, 0.1},
					},
				},
			),
		),
	), nil
}
