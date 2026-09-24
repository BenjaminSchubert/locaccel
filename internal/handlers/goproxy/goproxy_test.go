package goproxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/benjaminschubert/locaccel/internal/handlers/goproxy"
	"github.com/benjaminschubert/locaccel/internal/handlers/testutils"
	"github.com/benjaminschubert/locaccel/internal/httpclient"
)

func TestInstallGoPackages(t *testing.T) {
	t.Parallel()

	testutils.RunIntegrationTestsForHandler(
		t,
		"go",
		func(handler *http.ServeMux, client *httpclient.Client, upstreamCaches []*url.URL) {
			goproxy.RegisterHandler(
				"https://proxy.golang.org",
				"https://sum.golang.org",
				handler,
				client,
				upstreamCaches,
			)
		},
		func(t *testing.T, serverURL string) {
			t.Helper()

			cwd, err := os.Getwd()
			require.NoError(t, err)
			root := path.Dir(path.Dir(path.Dir(cwd)))

			testutils.Execute(
				t,
				"podman",
				"run",
				"--rm",
				"--interactive",
				"--network=host",
				"--dns=127.0.0.127",
				"--env=GOPROXY="+serverURL,
				"--env=GOSUMDB=sum.golang.org "+serverURL+"/sumdb",
				"--volume="+path.Join(root, "go.mod")+":/src/go.mod:z,ro",
				"--workdir=/src",
				"docker.io/golang:alpine",
				"go",
				"mod",
				"download",
				"-x",
				"github.com/mattn/go-colorable@v0.1.13",
			)
		},
		false,
		0,
		0,
		nil,
	)
}

func TestGoProxyRoutesSumdbProperly(t *testing.T) {
	t.Parallel()

	endpoints := make([]string, 0)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		endpoints = append(endpoints, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	require.NoError(t, err)

	logger := testutils.TestLogger(t, nil)
	handler := &http.ServeMux{}
	goproxy.RegisterHandler(
		"http://example.test",
		"http://sum.example.test/",
		handler,
		testutils.NewClient(t, false, logger),
		[]*url.URL{upstreamURL},
	)
	proxy, _ := testutils.NewServer(t, handler, "proxy", "proxy", logger)

	req, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		proxy.URL+"/sumdb/endpoint",
		nil,
	)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Empty(t, string(data))
	require.NoError(t, resp.Body.Close())

	require.Equal(t, []string{"/sumdb/endpoint"}, endpoints)
}
