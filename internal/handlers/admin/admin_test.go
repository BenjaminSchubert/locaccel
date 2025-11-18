package admin_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func getAdminServer(t *testing.T) (*httptest.Server, *httpclient.Cache) {
	t.Helper()

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
			&middleware.Statistics{},
		),
	)
	server := httptest.NewServer(
		middleware.ApplyAllMiddlewares(
			handler,
			"admin",
			logger,
			prometheus.NewPedanticRegistry(),
			&middleware.Statistics{},
		),
	)
	t.Cleanup(server.Close)

	return server, cache
}

func TestProxyLinuxDistributionPackageManagers(t *testing.T) {
	t.Parallel()

	server, _ := getAdminServer(t)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, resp.Body.Close())
	require.NoError(t, err)
	require.Contains(t, string(data), "Breakdown")
}

func TestDeletingUnknownKeysReturnProperError(t *testing.T) {
	t.Parallel()

	server, _ := getAdminServer(t)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		server.URL+"/cache/unknown",
		nil,
	)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestCanDeleteKeyWithNoFileSavedAnymore(t *testing.T) {
	t.Parallel()

	server, cache := getAdminServer(t)
	require.NoError(t, cache.New([]byte("mykey"), httpclient.CachedResponses{{ContentHash: "123"}}))

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		server.URL+"/cache/mykey",
		nil,
	)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestCanDeleteKey(t *testing.T) {
	t.Parallel()

	key := "GET+http://locaccel.test/admin"

	server, cache := getAdminServer(t)
	var hash string
	f := cache.SetupIngestion(
		io.NopCloser(bytes.NewReader([]byte("hello world!"))),
		func(h string) { hash = h },
		func() {},
		testutils.TestLogger(t),
	)
	_, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(t, cache.New([]byte(key), httpclient.CachedResponses{{ContentHash: hash}}))

	stats, err := cache.List(t.Context(), "locaccel.test", "test")
	require.NoError(t, err)
	require.Len(t, stats, 1)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodDelete,
		server.URL+"/cache/"+url.PathEscape(key),
		nil,
	)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	require.NoError(t, resp.Body.Close())
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	stats, err = cache.List(t.Context(), "locaccel.test", "test")
	require.NoError(t, err)
	require.Empty(t, stats)
}

func TestCanListEntriesPerHostname(t *testing.T) {
	t.Parallel()

	server, cache := getAdminServer(t)
	var hash string
	f := cache.SetupIngestion(
		io.NopCloser(bytes.NewReader([]byte("hello world!"))),
		func(h string) { hash = h },
		func() {},
		testutils.TestLogger(t),
	)
	_, err := io.ReadAll(f)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	require.NoError(
		t,
		cache.New(
			[]byte("GET+http://locaccel.test/admin"),
			httpclient.CachedResponses{{ContentHash: hash}},
		),
	)
	require.NoError(
		t,
		cache.New(
			[]byte("GET+http://locaccel.test/admin2"),
			httpclient.CachedResponses{{ContentHash: hash}},
		),
	)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		server.URL+"/hostname/"+url.PathEscape("locaccel.test"),
		nil,
	)
	require.NoError(t, err)
	resp, err := server.Client().Do(req)
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), "<td>http://locaccel.test/admin</td>")
	require.Contains(t, string(body), "<td>http://locaccel.test/admin2</td>")
}
