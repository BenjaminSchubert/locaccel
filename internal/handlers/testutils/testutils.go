package testutils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	tst "github.com/benjaminschubert/locaccel/internal/testutils"
	"github.com/benjaminschubert/locaccel/internal/units"
)

var (
	TestLogger                  = tst.TestLogger
	NewRequestCounterMiddleware = tst.NewRequestCounterMiddleware
)

func NewClient(t *testing.T, logger *zerolog.Logger) *httpclient.Client {
	t.Helper()

	client := &http.Client{
		Timeout: time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			MaxConnsPerHost:       20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}

	cache, err := httpclient.NewCache(
		path.Join(t.TempDir(), "cache"),
		units.Bytes{Bytes: 1024 * 1024},
		units.Bytes{Bytes: 10 * 1024 * 1024},
		logger,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cache.Close())
	})

	return httpclient.New(client, cache, logger, false, func(r *http.Request, status string) {})
}

func NewServer(
	t *testing.T,
	handler http.Handler,
	handlerName, handlerType string,
	counterMiddleware *tst.RequestCounterMiddleware,
	logger *zerolog.Logger,
) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(
		counterMiddleware.GetHandler(
			middleware.ApplyAllMiddlewares(
				handler,
				handlerName,
				logger,
				prometheus.NewPedanticRegistry(),
			),
			handlerType,
		),
	)
	t.Cleanup(srv.Close)
	return srv
}

func RunIntegrationTestsForHandler(
	t *testing.T,
	handlerName string,
	registerHandler func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL),
	test func(t *testing.T, serverURL string),
) {
	t.Helper()

	if testing.Short() {
		t.Skip("Integration test")
	}

	for _, useUpstreamCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("upstreamCache=%v", useUpstreamCache), func(t *testing.T) {
			t.Parallel()

			logger := TestLogger(t)
			var upstreams []*url.URL
			counterMiddleware := NewRequestCounterMiddleware(t)

			if useUpstreamCache {
				upstreamLogger := logger.With().Str("type", "upstream").Logger()
				handler := &http.ServeMux{}
				registerHandler(handler, NewClient(t, &upstreamLogger), nil)
				server := NewServer(
					t,
					handler,
					handlerName,
					"upstream",
					counterMiddleware,
					&upstreamLogger,
				)
				upstream, err := url.Parse(server.URL)
				require.NoError(t, err)
				upstreams = append(upstreams, upstream)

				localLogger := logger.With().Str("type", "local").Logger()
				logger = &localLogger
			}

			handler := &http.ServeMux{}
			registerHandler(handler, NewClient(t, logger), upstreams)
			server := NewServer(
				t,
				handler,
				handlerName,
				"upstream",
				counterMiddleware,
				logger,
			)

			test(t, server.URL)
		})
	}
}
