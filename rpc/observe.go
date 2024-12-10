package rpc

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/alphabill-org/alphabill/logger"
	"github.com/alphabill-org/alphabill/observability"
)

func metricsUpdater(mtr metric.Meter, node partitionNode, log *slog.Logger) func(ctx context.Context, method string, start time.Time, apiErr error) {
	callCnt, err := mtr.Int64Counter("calls", metric.WithDescription("How many times the endpoint has been called"))
	if err != nil {
		log.Error("creating calls counter", logger.Error(err))
		return func(context.Context, string, time.Time, error) { /* NOP */ }
	}
	callDur, err := mtr.Float64Histogram("duration",
		metric.WithDescription("How long it took to serve the request"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(25e-6, 50e-6, 100e-6, 200e-6, 400e-6, 800e-6, 0.0016, 0.01, 0.05, 0.1))
	if err != nil {
		log.Error("creating duration histogram", logger.Error(err))
		return func(context.Context, string, time.Time, error) { /* NOP */ }
	}

	fixedAttr := observability.Shard(node.PartitionID(), node.ShardID())
	statusOK := attribute.String("status", "ok")
	statusErr := attribute.String("status", "err")

	return func(ctx context.Context, method string, start time.Time, apiErr error) {
		methodAttr := attribute.String("method", method)
		statusAttr := statusOK
		if apiErr != nil {
			statusAttr = statusErr
		}
		callAttr := metric.WithAttributeSet(attribute.NewSet(methodAttr, statusAttr))

		callCnt.Add(ctx, 1, fixedAttr, callAttr)
		callDur.Record(ctx, time.Since(start).Seconds(), fixedAttr, callAttr)
	}
}

func metricsUpdaterTxReceived(mtr metric.Meter, node partitionNode, log *slog.Logger) func(ctx context.Context, txType uint16, apiErr error) {
	txReceived, err := mtr.Int64Counter(
		"tx.count",
		metric.WithDescription("Number of transactions received"),
		metric.WithUnit("{transaction}"),
	)
	if err != nil {
		log.Error("creating tx received counter", logger.Error(err))
		return func(ctx context.Context, txType uint16, apiErr error) { /* NOP */ }
	}

	fixedAttr := observability.Shard(node.PartitionID(), node.ShardID())
	statusOK := attribute.String("status", "ok")
	statusErr := attribute.String("status", "err")

	return func(ctx context.Context, txType uint16, apiErr error) {
		statusAttr := statusOK
		if apiErr != nil {
			statusAttr = statusErr
		}

		txReceived.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(attribute.Int("tx", int(txType)), statusAttr)), fixedAttr)
	}
}

/*
instrumentHTTP returns http middleware which instruments the incoming handler with two metrics:
  - number of calls: how many times the endpoint has been called;
  - request duration: how long it took to serve the request.
*/
func instrumentHTTP(mtr metric.Meter, log *slog.Logger) func(next http.Handler) http.Handler {
	callCnt, err := mtr.Int64Counter("calls", metric.WithDescription("How many times the endpoint has been called"))
	if err != nil {
		log.Error("creating calls counter", logger.Error(err))
		return passthroughMW
	}
	callDur, err := mtr.Float64Histogram("duration",
		metric.WithDescription("How long it took to serve the request"),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(100e-6, 200e-6, 400e-6, 800e-6, 0.0016, 0.01, 0.05, 0.1))
	if err != nil {
		log.Error("creating duration histogram", logger.Error(err))
		return passthroughMW
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var attr []attribute.KeyValue
			route := mux.CurrentRoute(req)
			if path, err := route.GetPathTemplate(); err != nil {
				log.WarnContext(req.Context(), "reading route path", logger.Error(err))
			} else {
				attr = append(attr, semconv.HTTPRoute(path))
			}

			start := time.Now()
			rsp := newstatusResponseWriter(w)
			next.ServeHTTP(rsp, req)

			attrSet := attribute.NewSet(append(attr, semconv.HTTPResponseStatusCode(rsp.statusCode))...)
			callCnt.Add(req.Context(), 1, metric.WithAttributeSet(attrSet))
			callDur.Record(req.Context(), time.Since(start).Seconds(), metric.WithAttributeSet(attrSet))
		})
	}
}

/*
passthroughMW is NOP middleware.
*/
func passthroughMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)
	})
}

/*
statusResponseWriter is a http.ResponseWriter wrapper which allows to capture
status code of the response.
https://www.alexedwards.net/blog/how-to-use-the-http-responsecontroller-type
*/
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newstatusResponseWriter(w http.ResponseWriter) *statusResponseWriter {
	return &statusResponseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (mw *statusResponseWriter) WriteHeader(statusCode int) {
	mw.ResponseWriter.WriteHeader(statusCode)

	if !mw.headerWritten {
		mw.statusCode = statusCode
		mw.headerWritten = true
	}
}

func (mw *statusResponseWriter) Write(b []byte) (int, error) {
	mw.headerWritten = true
	return mw.ResponseWriter.Write(b)
}

func (mw *statusResponseWriter) Unwrap() http.ResponseWriter {
	return mw.ResponseWriter
}
