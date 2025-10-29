package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/proxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestProxyLinuxDistributionPackageManagers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		image            string
		allowedUpstreams []string
		command          string
	}{
		{"debian:stable-slim", []string{"deb.debian.org"}, "apt-get update && apt-get install --assume-yes zsh"},
		{"ubuntu", []string{"archive.ubuntu.com", "security.ubuntu.com"}, "apt-get update && apt-get install --assume-yes zsh"},
	} {
		t.Run(tc.image, func(t *testing.T) {
			t.Parallel()

			testutils.RunIntegrationTestsForHandler(
				t,
				"proxy",
				func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
					proxy.RegisterHandler(tc.allowedUpstreams, handler, client, upstreamCaches)
				},
				func(t *testing.T, serverURL string) {
					t.Helper()

					testutils.Execute(
						t,
						"podman",
						"run",
						"--rm",
						"--interactive",
						"--network=host",
						"--dns=127.0.0.127",
						"--env=http_proxy="+serverURL,
						tc.image,
						"bash",
						"-c",
						tc.command,
					)
				},
				true,
			)
		})
	}
}

func TestProxyForbidsByDefault(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}
	proxy.RegisterHandler([]string{}, handler, testutils.NewClient(t, logger), nil)
	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(handler, "proxy", logger, prometheus.NewPedanticRegistry()),
	)
	defer server.Close()

	uri, err := url.Parse(server.URL)
	require.NoError(t, err)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(uri)}}

	resp, err := client.Get("http://perdu.com") //nolint:noctx
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func BenchmarkIntegrationProxy(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping integration benchmark")
	}

	logger := zerolog.New(zerolog.NewTestWriter(b)).Level(zerolog.WarnLevel)

	handler := &http.ServeMux{}
	proxy.RegisterHandler(
		[]string{"deb.debian.org"},
		handler,
		testutils.NewClient(b, &logger),
		nil,
	)

	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(
			handler,
			"proxy",
			&logger,
			nil,
		),
	)
	b.Cleanup(server.Close)

	download := func() {
		testutils.Execute(
			b,
			"podman",
			"run",
			"--rm",
			"--interactive",
			"--network=host",
			"--dns=127.0.0.127",
			"--env=http_proxy="+server.URL,
			"debian:stable-slim",
			"bash",
			"-c",
			"apt-get update && apt-get install --download-only --assume-yes firefox-esr",
		)
	}

	download()

	for b.Loop() {
		download()
	}
}
