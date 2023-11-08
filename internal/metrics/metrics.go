package metrics

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/metrics"
	"github.com/ethereum/go-ethereum/metrics/prometheus"
)

const prefix = "ab_"

var registry metrics.Registry

type Counter struct {
	metrics.Counter
}

func GetOrRegisterCounter(name string) *Counter {
	initMetrics()
	return &Counter{metrics.GetOrRegisterCounter(fmt.Sprintf("%s%s", prefix, name), registry)}
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
		return
	}
	metrics.Enabled = true
	registry = metrics.NewRegistry()
	metrics.DefaultRegistry = registry
}

func isMetricsEnabled() bool {
	// enable the metrics system by peeking into the command line args for the metrics flag.
	// Since we need this to run before any other code gets to create metrics, we'll actually do an ugly hack.
	for _, arg := range os.Args {
		flag := strings.TrimPrefix(arg, "--")
		if flag == "metrics" {
			return true
		}
	}
	return false
}
