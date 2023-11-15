package observability

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
)

func NOPMetrics() *Observability {
	return &Observability{mp: noop.NewMeterProvider()}
}

type Observability struct {
	mp metric.MeterProvider
}

func (o *Observability) Meter(name string, opts ...metric.MeterOption) metric.Meter {
	return o.mp.Meter(name)
}

func (o *Observability) MetricsHandler() http.Handler {
	return nil
}

func (o *Observability) PrometheusRegisterer() prometheus.Registerer {
	return nil
}
