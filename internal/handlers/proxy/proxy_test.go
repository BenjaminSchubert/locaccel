package proxy_test

import (
	"crypto/rand"
	"io"
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

	server := httptest.NewServer(middleware.ApplyAllMiddlewares(handler, "proxy", &logger, nil))
	b.Cleanup(server.Close)

	testutils.Execute(
		b,
		"podman",
		"run",
		"--rm",
		"--detach",
		"--network=host",
		"--dns=127.0.0.127",
		"--env=http_proxy="+server.URL,
		"--name=locaccel-test-debian",
		"debian:stable-slim",
		"sleep",
		"INFINITY",
	)
	defer testutils.Execute(b, "podman", "stop", "--time", "1", "locaccel-test-debian")

	download := func() {
		testutils.Execute(
			b,
			"podman",
			"exec",
			"-it",
			"locaccel-test-debian",
			"apt-get",
			"install",
			"--download-only",
			"--assume-yes",
			"firefox-esr",
		)
	}

	clean := func() {
		testutils.Execute(b, "podman", "exec", "-it", "locaccel-test-debian", "apt-get", "clean")
	}

	// Prepare the cache
	testutils.Execute(b, "podman", "exec", "-it", "locaccel-test-debian", "apt-get", "update")
	download()
	clean()

	for b.Loop() {
		download()

		b.StopTimer()
		clean()
		b.StartTimer()
	}
}

func BenchmarkProxy(b *testing.B) {
	data := make([]byte, 1024*1024)
	_, err := rand.Read(data)
	require.NoError(b, err)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Cache-Control", "max-age=1,must-revalidate")
		_, err := w.Write([]byte("Hello!"))
		assert.NoError(b, err)
	}))
	b.Cleanup(upstream.Close)

	upstreamUri, err := url.Parse(upstream.URL)
	require.NoError(b, err)

	logger := zerolog.New(zerolog.NewTestWriter(b)).Level(zerolog.ErrorLevel)

	handler := &http.ServeMux{}
	proxy.RegisterHandler(
		[]string{upstreamUri.Host},
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

	uri, err := url.Parse(server.URL)
	require.NoError(b, err)
	client := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(uri)}}

	req, err := http.NewRequestWithContext(b.Context(), http.MethodGet, upstream.URL, nil)
	require.NoError(b, err)

	for b.Loop() {
		resp, err := client.Do(req)
		require.NoError(b, err)
		require.Equal(b, http.StatusOK, resp.StatusCode)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(b, err)
		require.NoError(b, resp.Body.Close())
	}
}
