package metrics

import (
	"net/http"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/prometheus"
)

var registry metrics.Registry

type Counter struct {
	metrics.Counter
}

func GetOrRegisterCounter(name string) *Counter {
	initMetrics()
	logger.Debug("Creating new counter with name %v and registry %v", name, registry)
	return &Counter{metrics.GetOrRegisterCounter(name, registry)}
}

func Enabled() bool {
	return metrics.Enabled
}

func PrometheusHandler() http.Handler {
	return prometheus.Handler(metrics.DefaultRegistry)
}

func initMetrics() {
	if registry != nil {
		return
	}
	if !isMetricsEnabled() {
		logger.Debug("Metrics registry isn't initialised. Using 'nil' registry.")
		return
	}
	logger.Debug("Initialising metrics")
	metrics.Enabled = true
	registry = metrics.NewRegistry()
	metrics.DefaultRegistry = registry
}

func isMetricsEnabled() bool {
	// enable the metrics system by peeking into the command line args for the metrics flag.
	// Since we need this to run before any other code gets to create metrics, we'll actually do an ugly hack.
	for _, arg := range os.Args {
		flag := strings.TrimLeft(arg, "--")
		if flag == "metrics" {
			return true
		}
	}
	return false
}
