package proxy_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/proxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestProxyLinuxDistributionPackageManagers(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration test")
	}
	t.Parallel()

	for _, tc := range []struct {
		image             string
		allowed_upstreams []string
		command           string
	}{
		{"debian:stable-slim", []string{"deb.debian.org"}, "apt-get update && apt-get install --assume-yes zsh"},
		{"ubuntu", []string{"archive.ubuntu.com", "security.ubuntu.com"}, "apt-get update && apt-get install --assume-yes zsh"},
	} {
		t.Run(tc.image, func(t *testing.T) {
			t.Parallel()

			logger := testutils.TestLogger(t)

			handler := &http.ServeMux{}
			proxy.RegisterHandler(tc.allowed_upstreams, handler, testutils.NewClient(t, logger))
			server := httptest.NewServer(
				middleware.ApplyAllMiddlewares(
					handler,
					"proxy",
					logger,
					prometheus.NewPedanticRegistry(),
				),
			)
			defer server.Close()

			cmd := exec.Command( //nolint:gosec
				"podman",
				"run",
				"--rm",
				"--interactive",
				"--network=host",
				"--dns=127.0.0.127",
				"--env=http_proxy="+server.URL,
				tc.image,
				"bash",
				"-c",
				tc.command,
			)
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, string(output))
		})
	}
}

func TestProxyForbidsByDefault(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}
	proxy.RegisterHandler([]string{}, handler, testutils.NewClient(t, logger))
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
