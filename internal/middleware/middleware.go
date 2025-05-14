package middleware

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

func newLoggingMiddleware(handler http.Handler, logger *zerolog.Logger) http.Handler {
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

		l := hlog.FromRequest(req).WithLevel(level) //nolint:zerologlint
		if ua := req.Header.Get("User-Agent"); ua != "" {
			l = l.Str("usage-agent", ua)
		}
		l.
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

func ApplyAllMiddlewares(handler http.Handler, logger *zerolog.Logger) http.Handler {
	return newLoggingMiddleware(newTraceMiddleware(handler, logger), logger)
}
