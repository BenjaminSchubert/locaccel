package middleware

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func newLoggingMiddleware(
	handler http.Handler,
	logger *zerolog.Logger,
	statistics *Statistics,
) http.Handler {
	logHandler := hlog.NewHandler(*logger)

	correlationID := hlog.RequestIDHandler("id", "X-Locaccel-Correlation-ID")

	urlHandler := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log := zerolog.Ctx(r.Context())
			log.UpdateContext(func(c zerolog.Context) zerolog.Context {
				return c.Str("url", r.URL.Redacted())
			})
			next.ServeHTTP(w, r)
		})
	}

	access := hlog.AccessHandler(func(req *http.Request, status, size int, duration time.Duration) {
		level := zerolog.InfoLevel
		if status == 0 {
			level = zerolog.ErrorLevel
		} else if status >= http.StatusInternalServerError {
			level = zerolog.WarnLevel
		}

		ctx := req.Context()

		l := zerolog.Ctx(ctx).WithLevel(level) //nolint:zerologlint
		if ua := req.Header.Get("User-Agent"); ua != "" {
			l = l.Str("usage-agent", ua)
		}
		cacheState := GetCacheState(ctx)

		if size < 0 {
			panic("Unexpected negative size")
		}

		switch cacheState {
		case "N/A":
			statistics.UnCacheable.Add(1)
			statistics.BytesDownloaded.Add(uint64(size))
		case "hit":
			statistics.CacheHits.Add(1)
		case "miss":
			statistics.CacheMisses.Add(1)
			statistics.BytesDownloaded.Add(uint64(size))
		case "revalidated":
			statistics.Revalidated.Add(1)
		default:
			panic("Unexpected cache state: " + cacheState)
		}
		statistics.BytesServed.Add(uint64(size))

		l.
			Str("cache", cacheState).
			Str("ip", req.RemoteAddr).
			Str("method", req.Method).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("Processed request")
	})

	return logHandler(correlationID(access(urlHandler(handler))))
}

func newTraceMiddleware(next http.Handler, logger *zerolog.Logger) http.Handler {
	if logger.GetLevel() > zerolog.TraceLevel {
		logger.Debug().Msg("Tracing disabled, not adding trace middleware")
		return next
	}

	return http.HandlerFunc(func(respw http.ResponseWriter, req *http.Request) {
		headers := req.Header.Clone()
		headers.Del("Authorization")

		hlog.FromRequest(req).Trace().
			Any("headers", headers).
			Str("method", req.Method).
			Msg("Received request")
		defer func() {
			hlog.FromRequest(req).Trace().Any("headers", respw.Header()).Msg("Returned response")
		}()
		next.ServeHTTP(respw, req)
	})
}

func newMetricsMiddleware(
	next http.Handler,
	handlerName string,
	registry prometheus.Registerer,
) http.Handler {
	if registry == nil {
		return next
	}

	reg := prometheus.WrapRegistererWith(prometheus.Labels{"handler": handlerName}, registry)

	requestsTotal := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Tracks the number of HTTP requests.",
		}, []string{"method", "code", "cache"},
	)
	requestDuration := promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Tracks the latencies for HTTP requests.",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 60},
		},
		[]string{"method", "code", "cache"},
	)
	requestSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "Tracks the size of HTTP requests.",
		},
		[]string{"method", "code", "cache"},
	)
	responseSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_response_size_bytes",
			Help: "Tracks the size of HTTP responses.",
		},
		[]string{"method", "code", "cache"},
	)
	requestsInFlight := promauto.With(reg).NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Tracks how many requests are currently in flight",
		},
	)

	opts := promhttp.WithLabelFromCtx("cache", GetCacheState)

	next = promhttp.InstrumentHandlerResponseSize(responseSize, next, opts)
	next = promhttp.InstrumentHandlerRequestSize(requestSize, next, opts)
	next = promhttp.InstrumentHandlerDuration(requestDuration, next, opts)
	next = promhttp.InstrumentHandlerCounter(requestsTotal, next, opts)
	return promhttp.InstrumentHandlerInFlight(requestsInFlight, next)
}

func ApplyAllMiddlewares(
	handler http.Handler,
	serviceName string,
	logger *zerolog.Logger,
	registry prometheus.Registerer,
	statistics *Statistics,
) http.Handler {
	return StateHandler(
		newMetricsMiddleware(
			newLoggingMiddleware(newTraceMiddleware(handler, logger), logger, statistics),
			serviceName,
			registry,
		),
	)
}
