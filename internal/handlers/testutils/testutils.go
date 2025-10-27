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

func NewClient(tb testing.TB, logger *zerolog.Logger) *httpclient.Client {
	tb.Helper()

	client, _ := NewClientWithUnderlyingClient(tb, logger)
	return client
}

func NewClientWithUnderlyingClient(
	tb testing.TB,
	logger *zerolog.Logger,
) (cachingClient *httpclient.Client, httpClient *http.Client) {
	tb.Helper()

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
		false,
		func(r *http.Request, status string) {},
	), client
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
	supportsOfflineMode bool,
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

	t.Run("UpstreamDown", func(t *testing.T) {
		t.Parallel()

		if !supportsOfflineMode {
			t.Skip("Offline mode is not yet supported here")
		}

		logger := TestLogger(t)
		handler := &http.ServeMux{}
		counterMiddleware := NewRequestCounterMiddleware(t)
		cachingClient, httpClient := NewClientWithUnderlyingClient(t, logger)

		registerHandler(handler, cachingClient, nil)
		server := NewServer(
			t,
			handler,
			handlerName,
			"upstream",
			counterMiddleware,
			logger,
		)

		// Run the test, populating the cache
		test(t, server.URL)

		httpClient.Transport = &OfflineTransport{}

		// Run the test again, with the cache disconnected
		test(t, server.URL)
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

	cmd := exec.CommandContext(ctx, name, arg...)

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
