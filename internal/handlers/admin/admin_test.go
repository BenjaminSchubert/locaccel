package admin_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/admin"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
	"github.com/benjaminschubert/locaccel/internal/middleware"
)

func TestProxyLinuxDistributionPackageManagers(t *testing.T) {
	t.Parallel()

	logger := testutils.TestLogger(t)

	handler := &http.ServeMux{}

	cache, err := httpclient.NewCache(path.Join(t.TempDir(), "cache"), logger)
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, cache.Close())
	})

	admin.RegisterHandler(handler, cache)
	server := httptest.NewServer(middleware.ApplyAllMiddlewares(handler, logger))
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
