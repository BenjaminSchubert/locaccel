package middleware

import (
	"net/http"
	"net/url"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
)

type redactedURL struct {
	url *url.URL
}

func (r redactedURL) String() string {
	return r.url.Redacted()
}

func newLoggingMiddleware(handler http.Handler, logger zerolog.Logger) http.Handler {
	logHandler := hlog.NewHandler(logger)

	correlationID := hlog.RequestIDHandler("id", "X-Correlation-ID")

	access := hlog.AccessHandler(func(req *http.Request, status, size int, duration time.Duration) {
		level := zerolog.InfoLevel
		if status == 0 {
			level = zerolog.ErrorLevel
		} else if status >= http.StatusInternalServerError {
			level = zerolog.WarnLevel
		}

		hlog.FromRequest(req).WithLevel(level).
			Str("method", req.Method).
			Stringer("url", redactedURL{req.URL}).
			Int("status", status).
			Int("size", size).
			Dur("duration", duration).
			Msg("Processed request")
	})
	remote := hlog.RemoteAddrHandler("ip")
	userAgent := hlog.UserAgentHandler("user-agent")

	return logHandler(correlationID(access(remote(userAgent(handler)))))
}

func newTraceMiddleware(next http.Handler, logger zerolog.Logger) http.Handler {
	if logger.GetLevel() > zerolog.TraceLevel {
		logger.Debug().Msg("Tracing disabled, not adding trace middleware")
		return next
	}

	return http.HandlerFunc(func(respw http.ResponseWriter, req *http.Request) {
		// FIXME: can we avoid using Any for performance?
		hlog.FromRequest(req).Trace().
			// FIXME: drop authorization header
			Any("headers", req.Header).
			Str("method", req.Method).
			Stringer("url", redactedURL{req.URL}).
			Msg("Received request")
		defer func() {
			hlog.FromRequest(req).Trace().Any("headers", respw.Header()).Msg("Returned response")
		}()
		next.ServeHTTP(respw, req)
	})
}

func ApplyAllMiddlewares(handler http.Handler, logger zerolog.Logger) http.Handler {
	return newLoggingMiddleware(newTraceMiddleware(handler, logger), logger)
}
