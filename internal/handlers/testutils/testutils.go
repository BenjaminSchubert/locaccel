package testutils

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path"
	"strings"
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
	ErrUnableToContactUpstream  = errors.New("unable to contact server")
)

type OfflineTransport struct{}

func (o *OfflineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, ErrUnableToContactUpstream
}

func NewClient(tb testing.TB, isPrivate bool, logger *zerolog.Logger) *httpclient.Client {
	tb.Helper()

	client, _ := NewClientWithUnderlyingClient(tb, isPrivate, middleware.SetCacheState, logger)
	return client
}

func NewClientWithNotify(
	tb testing.TB,
	isPrivate bool,
	notify func(*http.Request, string),
	logger *zerolog.Logger,
) *httpclient.Client {
	tb.Helper()

	client, _ := NewClientWithUnderlyingClient(tb, isPrivate, notify, logger)
	return client
}

func NewClientWithUnderlyingClient(
	tb testing.TB,
	isPrivate bool,
	notify func(*http.Request, string),
	logger *zerolog.Logger,
) (cachingClient *httpclient.Client, httpClient *http.Client) {
	tb.Helper()

	client := &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			MaxConnsPerHost:       20,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   15 * time.Second,
			ExpectContinueTimeout: 5 * time.Second,
		},
	}

	cache, err := httpclient.NewCache(
		path.Join(tb.TempDir(), "cache"),
		units.Bytes{Bytes: 100 * 1024 * 1024},
		units.Bytes{Bytes: 1000 * 1024 * 1024},
		logger,
	)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		assert.NoError(tb, cache.Close())
	})

	return httpclient.New(
		client,
		cache,
		logger,
		isPrivate,
		notify,
		time.Now,
		time.Since,
	), client
}

func NewServer(
	t *testing.T,
	handler http.Handler,
	handlerName, handlerType string,
	counterMiddleware *tst.RequestCounterMiddleware,
	logger *zerolog.Logger,
) (*httptest.Server, *middleware.Statistics) {
	t.Helper()

	stats := &middleware.Statistics{}

	srv := httptest.NewServer(
		counterMiddleware.GetHandler(
			middleware.ApplyAllMiddlewares(
				handler,
				handlerName,
				logger,
				prometheus.NewPedanticRegistry(),
				stats,
			),
			handlerType,
		),
	)
	t.Cleanup(srv.Close)
	return srv, stats
}

func RunIntegrationTestsForHandler(
	t *testing.T,
	handlerName string,
	registerHandler func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL),
	test func(t *testing.T, serverURL string),
	needsPrivateCache bool,
	expectedCacheHitsOnInitialQuery uint64,
	expectedDeltaBetweenCacheHitsOnCachedQueryAndcacheMisses int64,
	expectedErrorMessagesWhenOffline []string,
) {
	t.Helper()

	if testing.Short() {
		t.Skip("Integration test")
	}

	for _, useUpstreamCache := range []bool{false, true} {
		t.Run(fmt.Sprintf("upstreamCache=%v", useUpstreamCache), func(t *testing.T) {
			t.Parallel()

			logger := TestLogger(t, nil)
			var upstreams []*url.URL
			counterMiddleware := NewRequestCounterMiddleware(t)

			if useUpstreamCache {
				upstreamLogger := logger.With().Str("type", "upstream").Logger()
				handler := &http.ServeMux{}
				registerHandler(handler, NewClient(t, needsPrivateCache, &upstreamLogger), nil)
				server, _ := NewServer(
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
			registerHandler(handler, NewClient(t, needsPrivateCache, logger), upstreams)
			server, stats := NewServer(
				t,
				handler,
				handlerName,
				"upstream",
				counterMiddleware,
				logger,
			)

			test(t, server.URL)

			assert.Positive(t, stats.BytesServed.Load())
			assert.Positive(t, stats.CacheMisses.Load())
			assert.Equal(t, expectedCacheHitsOnInitialQuery, stats.CacheHits.Load())
		})
	}

	t.Run("UpstreamDown", func(t *testing.T) {
		t.Parallel()

		logger := TestLogger(t, expectedErrorMessagesWhenOffline)
		handler := &http.ServeMux{}
		counterMiddleware := NewRequestCounterMiddleware(t)
		cachingClient, httpClient := NewClientWithUnderlyingClient(
			t,
			needsPrivateCache,
			middleware.SetCacheState,
			logger,
		)

		registerHandler(handler, cachingClient, nil)
		server, stats := NewServer(
			t,
			handler,
			handlerName,
			"upstream",
			counterMiddleware,
			logger,
		)

		// Run the test, populating the cache
		test(t, server.URL)

		assert.Positive(t, stats.BytesServed.Load())
		assert.Positive(t, stats.CacheMisses.Load())
		assert.Equal(t, expectedCacheHitsOnInitialQuery, stats.CacheHits.Load())

		httpClient.Transport = &OfflineTransport{}

		// Run the test again, with the cache disconnected
		test(t, server.URL)

		assert.Equal(
			t,
			uint64( //nolint:gosec
				int64( //nolint:gosec
					stats.CacheHits.Load(),
				)+expectedDeltaBetweenCacheHitsOnCachedQueryAndcacheMisses,
			),
			stats.CacheMisses.Load(),
		)
	})
}

func Execute(tb testing.TB, name string, arg ...string) {
	tb.Helper()

	baseCtx := tb.Context()
	if tb.Context().Err() != nil && errors.Is(tb.Context().Err(), context.Canceled) {
		baseCtx = context.Background()
	}

	ctx, cancel := context.WithTimeout(baseCtx, 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, arg...) //nolint:gosec

	output, err := cmd.CombinedOutput()
	displayCmd := strings.Join(append([]string{name}, arg...), " ")
	require.NotErrorIs(
		tb,
		err,
		context.DeadlineExceeded,
		"command '%s' timed out after 5m:\n-----\n%s\n-----",
		displayCmd,
		output,
	)
	require.NotErrorIs(
		tb,
		ctx.Err(),
		context.DeadlineExceeded,
		"command '%s' timed out after 5m:\n-----\n%s\n-----",
		displayCmd,
		output,
	)
	require.NoError(tb, err, "Command '%s' failed:\n-----\n%s\n-----", displayCmd, output)
}
