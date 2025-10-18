package admin_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/config"
	"github.com/benjaminschubert/locaccel/internal/handlers/admin"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
	"github.com/benjaminschubert/locaccel/internal/units"
)

func TestProxyLinuxDistributionPackageManagers(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}

	cache, err := httpclient.NewCache(
		path.Join(t.TempDir(), "cache"),
		units.Bytes{Bytes: 100},
		units.Bytes{Bytes: 1000},
		logger,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cache.Close())
	})

	require.NoError(
		t,
		admin.RegisterHandler(
			handler,
			cache,
			config.Default(func(s string) (string, bool) { return "", false }),
		),
	)
	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(handler, "admin", logger, prometheus.NewPedanticRegistry()),
	)
	defer server.Close()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)
	require.Contains(t, string(data), "Breakdown")
}
